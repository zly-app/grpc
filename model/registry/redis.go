package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/zly-app/component/redis"
	"github.com/zly-app/zapp/component/gpool"
	"github.com/zly-app/zapp/log"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"

	"github.com/zly-app/grpc/foundation/addr"
)

// RedisRegistry 拥有本进程已注册实例的本地副本与续租规则；权威状态在 Redis hash。
type RedisRegistry struct {
	client  redis.UniversalClient
	ticker  *time.Ticker
	stop    chan struct{}
	mx      sync.Mutex
	servers map[string]*RegServer
}

// NewRedisRegistry 建注册器并启动续租循环；ctx 作为后台任务的父上下文。
func NewRedisRegistry(_ context.Context, address string) (*RedisRegistry, error) {
	client, err := redis.GetClient(address)
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

// Register 是唯一施放入口：写 hash → 发注册信号 → 记本地副本，三步不拆给外部。
// 信号发布失败必须上抛，否则「状态变了、通知没发」，下游会静默失明。
func (r *RedisRegistry) Register(ctx context.Context, serverName string, a *addr.Info) error {
	seqNo, err := r.client.Incr(ctx, SeqKey(serverName)).Result()
	if err != nil {
		log.Error(ctx, "申请注册序号失败", zap.String("serverName", serverName), zap.Error(err))
		return err
	}

	name := fmt.Sprintf("%s.%d", serverName, seqNo)
	if a.Name != "" {
		name = fmt.Sprintf("%s.%d", a.Name, seqNo)
	}
	reg := &RegServer{
		SeqNo:    int(seqNo),
		Name:     name,
		Endpoint: a.Endpoint,
		Weight:   a.Weight,
	}

	if err := r.writeOne(ctx, serverName, reg); err != nil {
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
// 任一失败显式记录，由发现侧 ReDiscoverInterval 的主动重发现兜底（环的终止保障）。
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

	key, field := RegKey(serverName), RegField(reg.SeqNo)
	if err := r.client.HDel(ctx, key, field).Err(); err != nil {
		log.Error(ctx, "取消注册失败", zap.String("serverName", serverName), zap.Error(err))
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

func (r *RedisRegistry) writeOne(ctx context.Context, serverName string, reg *RegServer) error {
	reg.Deadline = time.Now().Unix() + RegistryExpire
	data, _ := sonic.MarshalString(reg)
	if err := r.client.HSet(ctx, RegKey(serverName), RegField(reg.SeqNo), data).Err(); err != nil {
		log.Error(ctx, "写入注册记录失败", zap.String("serverName", serverName), zap.Error(err))
		return err
	}
	return nil
}

func (r *RedisRegistry) leaseAll() {
	ctx, span := utils.Trace.StartSpan(context.Background(), "ReRegistryGrpcServer")
	defer utils.Trace.EndSpan(span)

	r.mx.Lock()
	snapshot := make(map[string]*RegServer, len(r.servers))
	for k, v := range r.servers {
		snapshot[k] = v
	}
	r.mx.Unlock()

	for k, v := range snapshot {
		serverName, reg := k, v
		gpool.GetDefGPool().Go(func() error {
			return r.writeOne(ctx, serverName, reg)
		}, func(err error) {
			if err != nil {
				log.Error(ctx, "续租注册记录失败", zap.String("serverName", serverName), zap.Error(err))
			}
		})
	}
}
