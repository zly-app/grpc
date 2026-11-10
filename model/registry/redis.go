package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	rredis "github.com/redis/go-redis/v9"
	"github.com/bytedance/sonic"
	"github.com/zly-app/component/redis"
	"github.com/zly-app/zapp/component/gpool"
	"github.com/zly-app/zapp/log"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"

	"github.com/zly-app/grpc/foundation/addr"
)

// unregisterTimeout 限制注销动作的最长耗时：注销是「加速下线」，不能阻塞进程退出。
const unregisterTimeout = 3 * time.Second

// RedisRegistry 拥有本进程已注册实例的本地副本与续租规则；权威状态在 Redis hash。
// 每条记录的有效期由字段级 TTL（HSETEX）表达，过期由 Redis 负责，客户端不需要删除。
type RedisRegistry struct {
	client  redis.UniversalClient
	ticker  *time.Ticker
	stop    chan struct{}
	mx      sync.Mutex
	servers map[string]*RegServer
}

// NewRedisRegistry 建注册器并启动续租循环；ctx 作为后台任务的父上下文。
func NewRedisRegistry(_ context.Context, address string, creator redis.Creator) (*RedisRegistry, error) {
	client, err := creator.GetClient(address)
	if err != nil {
		return nil, err
	}
	r := &RedisRegistry{
		client:  client,
		ticker:  time.NewTicker(time.Second * ReRegInterval),
		stop:    make(chan struct{}),
		servers: make(map[string]*RegServer),
	}
	go r.leaseLoop()
	return r, nil
}

// Register 是唯一施放入口：写带 TTL 的 hash field → 发注册信号 → 记本地副本，三步不拆给外部。
// 信号发布失败必须上抛，否则「状态变了、通知没发」，下游会静默失明。
func (r *RedisRegistry) Register(ctx context.Context, serverName string, a *addr.Info) error {
	if a == nil || a.Endpoint == "" {
		return fmt.Errorf("注册地址为空: %s", serverName)
	}
	reg := &RegServer{
		Name:     regName(a),
		Endpoint: RegField(a.Endpoint),
		Weight:   a.Weight,
	}

	if err := r.claim(ctx, serverName, reg); err != nil {
		return err
	}

	text, _ := sonic.MarshalString(&RegSignal{Reg: reg, IsUnReg: false})
	if err := r.client.Publish(ctx, SignalKey(serverName), text).Err(); err != nil {
		log.Error(ctx, "发布注册信号失败", zap.String("serverName", serverName), zap.Error(err))
		return fmt.Errorf("发布注册信号失败: %w", err)
	}

	r.mx.Lock()
	r.servers[serverName] = reg
	r.mx.Unlock()
	return nil
}

// Unregister 是唯一施放入口：删 hash 与发取消信号在同一调用内完成。
// 删除只是「加速下线」；失败由字段 TTL 兜底，因此超时后放弃而不是阻塞退出。
func (r *RedisRegistry) Unregister(ctx context.Context, serverName string) {
	r.mx.Lock()
	reg, ok := r.servers[serverName]
	if ok {
		delete(r.servers, serverName)
	}
	r.mx.Unlock()
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, unregisterTimeout)
	defer cancel()

	key, field := RegKey(serverName), RegField(reg.Endpoint)
	if err := r.client.HDel(ctx, key, field).Err(); err != nil {
		log.Error(ctx, "取消注册失败（注册记录将在 TTL 到期后自动失效）",
			zap.String("serverName", serverName), zap.String("endpoint", field), zap.Error(err))
	}

	text, _ := sonic.MarshalString(&RegSignal{Reg: reg, IsUnReg: true})
	if err := r.client.Publish(ctx, SignalKey(serverName), text).Err(); err != nil {
		log.Error(ctx, "发布取消注册信号失败", zap.String("serverName", serverName), zap.Error(err))
	}
}

// Close 停续租循环。
func (r *RedisRegistry) Close() {
	r.ticker.Stop()
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}

func (r *RedisRegistry) leaseLoop() {
	for {
		select {
		case <-r.ticker.C:
			r.leaseAll()
		case <-r.stop:
			return
		}
	}
}

// regName 生成进入一致性 hash 环的节点名：以 endpoint 结尾保证「同实例跨重启不变」，
// PublishName 只作可读前缀，不参与唯一性。
func regName(a *addr.Info) string {
	if a.Name == "" || a.Name == a.Endpoint {
		return RegField(a.Endpoint)
	}
	return a.Name + "@" + RegField(a.Endpoint)
}

// claim 首次落地一条记录：写值与设 TTL 用一条 HSETEX 完成，不需要 Lua。
// 先用 FNX 探测该 endpoint 是否已被占：被占时要区分「自己重启接管」与
// 「配置错误导致两个活实例共用同一 endpoint」，后者靠 TTL 是否仍在被续来识别。
func (r *RedisRegistry) claim(ctx context.Context, serverName string, reg *RegServer) error {
	key, field := RegKey(serverName), RegField(reg.Endpoint)
	data, _ := sonic.MarshalString(reg)

	opt := &rredis.HSetEXOptions{
		Condition:      rredis.HSetEXFNX,
		ExpirationType: rredis.HSetEXExpirationEX,
		ExpirationVal:  RegistryExpire,
	}
	n, err := r.client.HSetEXWithArgs(ctx, key, opt, field, data).Result()
	if err != nil {
		log.Error(ctx, "写入注册记录失败", zap.String("serverName", serverName), zap.String("endpoint", field), zap.Error(err))
		return err
	}
	if n == 1 {
		return nil
	}

	r.reportTakeover(ctx, key, field)

	opt.Condition = ""
	if err := r.client.HSetEXWithArgs(ctx, key, opt, field, data).Err(); err != nil {
		log.Error(ctx, "覆盖注册记录失败", zap.String("serverName", serverName), zap.String("endpoint", field), zap.Error(err))
		return err
	}
	return nil
}

// reportTakeover 记录「该 endpoint 上已有记录，本次是接管」。
// 记录里没有可以稳定标识进程的字段（这正是弃用自增 seq 的原因），
// 因此「自己重启」与「两个活实例共用同一 endpoint（配置错误）」在协议上无法区分，
// 统一按接管处理，是否冲突以「注册表条目数 vs 实际实例数」比对为准。
func (r *RedisRegistry) reportTakeover(ctx context.Context, key, field string) {
	ttl, err := r.client.HTTL(ctx, key, field).Result()
	if err != nil || len(ttl) != 1 {
		return
	}
	log.Info(ctx, "接管已存在的注册记录（同 endpoint 重启与实例标识冲突无法区分，冲突以条目数与实例数比对为准）",
		zap.String("key", key), zap.String("endpoint", field), zap.Int64("ttl", ttl[0]))
}

// touch 续租一条记录：重写值与重设 TTL 同为一条 HSETEX，无需判断记录是否已过期。
func (r *RedisRegistry) touch(ctx context.Context, serverName string, reg *RegServer) error {
	key, field := RegKey(serverName), RegField(reg.Endpoint)
	data, _ := sonic.MarshalString(reg)
	opt := &rredis.HSetEXOptions{
		ExpirationType: rredis.HSetEXExpirationEX,
		ExpirationVal:  RegistryExpire,
	}
	if err := r.client.HSetEXWithArgs(ctx, key, opt, field, data).Err(); err != nil {
		log.Error(ctx, "续租注册记录失败", zap.String("serverName", serverName), zap.String("endpoint", field), zap.Error(err))
		return err
	}
	return nil
}

func (r *RedisRegistry) leaseAll() {
	ctx, span := utils.Trace.StartSpan(context.Background(), "ReRegistryGrpcServer")
	defer utils.Trace.EndSpan(span)

	r.mx.Lock()
	snapshot := make(map[string]RegServer, len(r.servers))
	for k, v := range r.servers {
		snapshot[k] = *v // 值拷贝：续租不再改共享对象，避免与 Unregister 并发读写
	}
	r.mx.Unlock()

	for k, v := range snapshot {
		serverName, reg := k, v
		gpool.GetDefGPool().Go(func() error {
			return r.touch(ctx, serverName, &reg)
		}, func(err error) {
			if err != nil {
				log.Error(ctx, "续租注册记录失败", zap.String("serverName", serverName), zap.Error(err))
			}
		})
	}
}
