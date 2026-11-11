package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	rredis "github.com/redis/go-redis/v9"
	"github.com/zly-app/component/redis"
	"github.com/zly-app/zapp/log"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"

	"github.com/zly-app/grpc/foundation/addr"
	"github.com/zly-app/grpc/foundation/port"
	"github.com/zly-app/grpc/model/endpoint"
)

const (
	// writeTimeout 限制单条写入命令的耗时：任何一次注册动作都不允许无限挂起。
	writeTimeout = 3 * time.Second
	// unregisterTimeout 限制注销动作：注销是「加速下线」，不能阻塞进程退出。
	unregisterTimeout = 3 * time.Second
)

// RedisRegistry 拥有本进程已注册实例的本地副本与续租规则；权威状态在 Redis hash。
// 每条记录的有效期由字段级 TTL（HSETEX）表达，过期由 Redis 负责，客户端不需要删除。
type RedisRegistry struct {
	client   redis.UniversalClient
	parent   context.Context
	ticker   *time.Ticker
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	mx      sync.Mutex
	servers map[string]endpoint.Record
}

var _ port.Registry = (*RedisRegistry)(nil)

// NewRedisRegistry 建注册器并启动续租循环；ctx 作为后台任务的父上下文。
func NewRedisRegistry(ctx context.Context, address string, creator redis.Creator) (*RedisRegistry, error) {
	client, err := creator.GetClient(address)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r := &RedisRegistry{
		client:  client,
		parent:  ctx,
		ticker:  time.NewTicker(endpoint.LeaseInterval * time.Second),
		stop:    make(chan struct{}),
		servers: make(map[string]endpoint.Record),
	}
	r.wg.Add(1)
	go r.leaseLoop()
	return r, nil
}

// Register 是唯一施放入口：写带 TTL 的字段 → 发注册信号 → 记本地副本。
//
// 正常路径一条 HSETEX(FNX) 落库；返回 0 说明该 endpoint 上已有记录（本进程重启接管，
// 或配置错误导致两个活实例共用同一标识），此时覆盖写入并给出倾向性判断。
//
// 通知失败只记日志不返回错误：发现侧有主动重发现兜底，通知通道的短暂故障
// 不应该把「服务本身健康、注册也已落库」的进程挡在启动之外。
func (r *RedisRegistry) Register(ctx context.Context, serverName string, a *addr.Info) error {
	if a == nil || a.Endpoint == "" {
		return fmt.Errorf("注册地址为空: %s", serverName)
	}
	rec := endpoint.Record{
		Name:     endpoint.RecordName(a.Name, a.Endpoint),
		Endpoint: endpoint.Field(a.Endpoint),
		Weight:   a.Weight,
	}

	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	n, err := r.write(ctx, serverName, rec, rredis.HSetEXFNX)
	if err != nil {
		log.Error(ctx, "写入注册记录失败",
			zap.String("serverName", serverName), zap.String("endpoint", rec.Endpoint), zap.Error(err))
		return err
	}
	if n == 0 {
		r.reportTakeover(ctx, serverName, rec)
		if _, err := r.write(ctx, serverName, rec, ""); err != nil {
			log.Error(ctx, "覆盖注册记录失败",
				zap.String("serverName", serverName), zap.String("endpoint", rec.Endpoint), zap.Error(err))
			return err
		}
	}

	r.publish(ctx, serverName, rec, false)

	r.mx.Lock()
	r.servers[serverName] = rec
	r.mx.Unlock()
	return nil
}

// Unregister 是唯一施放入口：删 hash 与发取消信号在同一调用内完成。
// 删除只是「加速下线」；失败由字段 TTL 兜底，因此超时后放弃而不是阻塞退出。
func (r *RedisRegistry) Unregister(ctx context.Context, serverName string) {
	r.mx.Lock()
	rec, ok := r.servers[serverName]
	if ok {
		delete(r.servers, serverName)
	}
	r.mx.Unlock()
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, unregisterTimeout)
	defer cancel()

	if err := r.client.HDel(ctx, endpoint.RegKey(serverName), rec.Endpoint).Err(); err != nil {
		log.Error(ctx, "取消注册失败（注册记录将在 TTL 到期后自动失效）",
			zap.String("serverName", serverName), zap.String("endpoint", rec.Endpoint), zap.Error(err))
	}

	r.publish(ctx, serverName, rec, true)
}

// Close 停续租循环并等它退出；可重复调用。
func (r *RedisRegistry) Close() {
	r.ticker.Stop()
	r.stopOnce.Do(func() { close(r.stop) })
	r.wg.Wait()
}

func (r *RedisRegistry) leaseLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ticker.C:
			r.leaseAll()
		case <-r.stop:
			return
		case <-r.parent.Done():
			return
		}
	}
}

// leaseAll 对全部已注册实例并发续租一次。
//
// 心跳自己调度，不借用共享任务池：池被业务任务占满时会把续租推迟到 TTL 之后，
// 表现为「进程活着但注册记录消失」。
func (r *RedisRegistry) leaseAll() {
	ctx, span := utils.Trace.StartSpan(r.parent, "ReRegistryGrpcServer")
	defer utils.Trace.EndSpan(span)

	r.mx.Lock()
	snapshot := make(map[string]endpoint.Record, len(r.servers))
	for k, v := range r.servers {
		snapshot[k] = v
	}
	r.mx.Unlock()

	var wg sync.WaitGroup
	for name, rec := range snapshot {
		wg.Add(1)
		go func(name string, rec endpoint.Record) {
			defer wg.Done()
			r.leaseOne(ctx, name, rec)
		}(name, rec)
	}
	wg.Wait()
}

// leaseOne 用 FXX 续租：只在字段仍存在时刷新值与 TTL。
//
// 返回 0 说明记录已因 TTL 过期（或 Redis 侧被清），必须重建并补发注册信号。
// 否则会出现「服务活着、注册表也重建了、但发现侧看不到」的黑洞——
// 发现侧是纯只读的，只有信号或重发现能推进状态。
func (r *RedisRegistry) leaseOne(ctx context.Context, serverName string, rec endpoint.Record) {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	n, err := r.write(ctx, serverName, rec, rredis.HSetEXFXX)
	if err != nil {
		log.Error(ctx, "续租注册记录失败",
			zap.String("serverName", serverName), zap.String("endpoint", rec.Endpoint), zap.Error(err))
		return
	}
	if n > 0 {
		return
	}

	log.Warn(ctx, "注册记录已过期，重建并补发注册信号",
		zap.String("serverName", serverName), zap.String("endpoint", rec.Endpoint))
	if _, err := r.write(ctx, serverName, rec, ""); err != nil {
		log.Error(ctx, "重建注册记录失败",
			zap.String("serverName", serverName), zap.String("endpoint", rec.Endpoint), zap.Error(err))
		return
	}
	r.publish(ctx, serverName, rec, false)
}

// write 写一条记录：值与字段级 TTL 同一条 HSETEX 完成。
// cond 为空表示无条件覆盖（重启接管）；FNX 只在字段不存在时写；FXX 只在字段存在时写（续租）。
func (r *RedisRegistry) write(ctx context.Context, serverName string, rec endpoint.Record,
	cond rredis.HSetEXCondition) (int64, error) {
	data, err := sonic.MarshalString(rec)
	if err != nil {
		return 0, fmt.Errorf("序列化注册记录失败: %w", err)
	}
	opt := &rredis.HSetEXOptions{
		Condition:      cond,
		ExpirationType: rredis.HSetEXExpirationEX,
		ExpirationVal:  endpoint.RecordTTL,
	}
	return r.client.HSetEXWithArgs(ctx, endpoint.RegKey(serverName), opt, rec.Endpoint, data).Result()
}

// publish 发一次状态变更通知；失败只记日志，由发现侧的主动重发现兜底。
func (r *RedisRegistry) publish(ctx context.Context, serverName string, rec endpoint.Record, isUnReg bool) {
	text, err := sonic.MarshalString(&endpoint.Signal{Record: &rec, IsUnReg: isUnReg})
	if err != nil {
		log.Error(ctx, "序列化注册信号失败", zap.String("serverName", serverName), zap.Error(err))
		return
	}
	if err := r.client.Publish(ctx, endpoint.SignalKey(serverName), text).Err(); err != nil {
		log.Error(ctx, "发布注册信号失败（依赖发现侧主动重发现兜底）",
			zap.String("serverName", serverName), zap.String("endpoint", rec.Endpoint),
			zap.Bool("isUnReg", isUnReg), zap.Error(err))
	}
}

// reportTakeover 说明该 endpoint 上已有注册记录：可能是本进程未优雅退出的残留，
// 也可能是另一个活实例共用同一标识（配置错误）。
//
// 两者在协议上无法区分——崩溃前最后一次续租总落在 0~10s 内，TTL 必然在 (20,30]，
// 与「另一个活实例正在续租」的 TTL 分布完全重叠，用 TTL 判定会 100% 误报。
// 所以这里只给警告与核对方法，不据此判错，也不阻止接管。
func (r *RedisRegistry) reportTakeover(ctx context.Context, serverName string, rec endpoint.Record) {
	ttl, err := r.client.HTTL(ctx, endpoint.RegKey(serverName), rec.Endpoint).Result()
	if err != nil || len(ttl) != 1 {
		log.Warn(ctx, "该 endpoint 已有注册记录，本次接管覆盖",
			zap.String("serverName", serverName), zap.String("endpoint", rec.Endpoint))
		return
	}
	log.Warn(ctx, "该 endpoint 已有注册记录，本次接管覆盖；若注册表条目数与实际实例数不符，"+
		"说明多个活实例共用同一标识（检查 PublishAddress / Frame.Instance 是否每实例唯一）",
		zap.String("serverName", serverName), zap.String("endpoint", rec.Endpoint), zap.Int64("ttl", ttl[0]))
}
