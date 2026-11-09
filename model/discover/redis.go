package discover

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	rredis "github.com/redis/go-redis/v9"
	"github.com/zly-app/component/redis"
	"github.com/zly-app/zapp/component/gpool"
	"github.com/zly-app/zapp/log"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"
	gresolver "google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/foundation/addr"
	fresolver "github.com/zly-app/grpc/foundation/resolver"
	"github.com/zly-app/grpc/model/registry"
)

// RedisDiscover 拥有每个服务的地址表与广播器；订阅信号与定时重发现是两条
// 互补的更新路径，都只推状态、不触发新的发现请求（环的终止保障）。
type RedisDiscover struct {
	client redis.UniversalClient
	sub    *rredis.PubSub
	ticker *time.Ticker
	stop   chan struct{}

	mx  sync.Mutex
	res map[string]*serverEntry
}

// serverEntry 是单个服务的地址表 + 广播器，数据由本结构独占。
type serverEntry struct {
	b       *fresolver.Builder
	regData []*registry.RegServer
	upTime  int64
	mx      sync.Mutex
}

func (e *serverEntry) remove(seqNo int) {
	e.mx.Lock()
	defer e.mx.Unlock()

	next := make([]*registry.RegServer, 0, len(e.regData))
	for _, v := range e.regData {
		if v.SeqNo != seqNo {
			next = append(next, v)
		}
	}
	if len(next) == len(e.regData) {
		return
	}
	e.update(next)
}

func (e *serverEntry) add(reg *registry.RegServer) {
	e.mx.Lock()
	defer e.mx.Unlock()

	for _, v := range e.regData {
		if v.SeqNo == reg.SeqNo {
			return
		}
	}
	e.regData = append(e.regData, reg)
	e.update(e.regData)
}

func (e *serverEntry) update(regData []*registry.RegServer) {
	e.regData = regData
	e.b.UpdateState(gresolver.State{Addresses: makeAddress(regData)})
	e.upTime = time.Now().Unix()
}

func (e *serverEntry) tryUpdate(regData []*registry.RegServer) {
	e.mx.Lock()
	defer e.mx.Unlock()

	if len(regData) != len(e.regData) {
		e.update(regData)
		return
	}
	for i := range regData {
		if regData[i].SeqNo != e.regData[i].SeqNo {
			e.update(regData)
			return
		}
	}
	e.upTime = time.Now().Unix()
}

// NewRedisDiscover 建发现器并启动订阅与重发现循环。
func NewRedisDiscover(ctx context.Context, address string) (*RedisDiscover, error) {
	client, err := redis.GetClient(address)
	if err != nil {
		return nil, err
	}
	r := &RedisDiscover{
		client: client,
		sub:    client.Subscribe(ctx),
		ticker: time.NewTicker(time.Second * registry.ReDiscoverInterval),
		stop:   make(chan struct{}),
		res:    make(map[string]*serverEntry),
	}
	go r.consumeSignals()
	go r.reDiscoverLoop()
	return r, nil
}

// Builder 返回该服务的 resolver.Builder；无地址时显式报错。
func (r *RedisDiscover) Builder(ctx context.Context, serverName string) (gresolver.Builder, error) {
	r.mx.Lock()
	entry, ok := r.res[serverName]
	if ok {
		r.mx.Unlock()
		if len(entry.regData) == 0 {
			return nil, fmt.Errorf("server %s not found router", serverName)
		}
		return entry.b, nil
	}
	r.mx.Unlock()

	regData, err := r.discoverOne(ctx, serverName)
	if err != nil {
		return nil, err
	}
	address := makeAddress(regData)
	b := fresolver.NewBuilderWithScheme(registry.TypeRedis)
	b.InitialState(gresolver.State{Addresses: address})
	entry = &serverEntry{b: b, regData: regData, upTime: time.Now().Unix()}

	if err := r.sub.Subscribe(ctx, registry.SignalKey(serverName)); err != nil {
		log.Error(ctx, "订阅注册信号失败", zap.String("serverName", serverName), zap.Error(err))
		return nil, err
	}

	r.mx.Lock()
	r.res[serverName] = entry
	r.mx.Unlock()
	if len(regData) == 0 {
		return nil, fmt.Errorf("server %s not found router", serverName)
	}
	return b, nil
}

// Close 关订阅与重发现循环。
func (r *RedisDiscover) Close() {
	_ = r.sub.Close()
	r.ticker.Stop()
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}

func (r *RedisDiscover) consumeSignals() {
	for msg := range r.sub.Channel() {
		if !strings.HasPrefix(msg.Channel, registry.KeyRegSignal) {
			continue
		}
		serverName := msg.Channel[len(registry.KeyRegSignal):]

		r.mx.Lock()
		entry, ok := r.res[serverName]
		r.mx.Unlock()
		if !ok {
			continue
		}

		signal := registry.RegSignal{}
		if err := sonic.UnmarshalString(msg.Payload, &signal); err != nil || signal.Reg == nil {
			continue // 异常信号显式丢弃，不推进状态
		}
		if signal.IsUnReg {
			entry.remove(signal.Reg.SeqNo)
			continue
		}
		entry.add(signal.Reg)
	}
}

func (r *RedisDiscover) reDiscoverLoop() {
	for {
		select {
		case <-r.ticker.C:
			r.reDiscoverAll()
		case <-r.stop:
			return
		}
	}
}

func (r *RedisDiscover) discoverOne(ctx context.Context, serverName string) ([]*registry.RegServer, error) {
	key := registry.RegKey(serverName)
	data, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		log.Error(ctx, "读取注册地址失败", zap.String("serverName", serverName), zap.Error(err))
		return nil, err
	}

	delSeq := make([]string, 0)
	nowUnix := time.Now().Unix()
	ret := make([]*registry.RegServer, 0)
	for seqNo, text := range data {
		reg := registry.RegServer{}
		if err := sonic.UnmarshalString(text, &reg); err != nil {
			log.Warn(ctx, "注册记录反序列化失败", zap.String("serverName", serverName), zap.Error(err))
			delSeq = append(delSeq, seqNo)
			continue
		}
		if nowUnix >= reg.Deadline {
			if nowUnix-reg.Deadline >= registry.DelRegDataThanTimeOverdue {
				log.Warn(ctx, "注册记录超长过期", zap.String("serverName", serverName), zap.String("seq", seqNo))
				delSeq = append(delSeq, seqNo)
			}
			continue
		}
		ret = append(ret, &reg)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].SeqNo < ret[j].SeqNo })

	if len(delSeq) > 0 {
		if err := r.client.HDel(ctx, key, delSeq...).Err(); err != nil {
			log.Error(ctx, "清理过期注册记录失败", zap.String("serverName", serverName), zap.Error(err))
		}
	}
	return ret, nil
}

func makeAddress(regData []*registry.RegServer) []gresolver.Address {
	list := make([]gresolver.Address, 0, len(regData))
	for _, a := range regData {
		ra := gresolver.Address{Addr: a.Endpoint}
		ra = addr.Set(ra, &addr.Info{Name: a.Name, Endpoint: a.Endpoint, Weight: a.Weight})
		list = append(list, ra)
	}
	return list
}

func (r *RedisDiscover) reDiscoverOne(ctx context.Context, serverName string, entry *serverEntry) error {
	if time.Now().Unix()-entry.upTime < registry.ReDiscoverInterval/2 {
		return nil
	}
	regData, err := r.discoverOne(ctx, serverName)
	if err != nil {
		return err
	}
	entry.tryUpdate(regData)
	return nil
}

func (r *RedisDiscover) reDiscoverAll() {
	ctx, span := utils.Trace.StartSpan(context.Background(), "ReDiscoverGrpcServer")
	defer utils.Trace.EndSpan(span)

	r.mx.Lock()
	snapshot := make(map[string]*serverEntry, len(r.res))
	for k, v := range r.res {
		snapshot[k] = v
	}
	r.mx.Unlock()

	for k, v := range snapshot {
		serverName, entry := k, v
		gpool.GetDefGPool().Go(func() error {
			return r.reDiscoverOne(ctx, serverName, entry)
		}, func(err error) {
			if err != nil {
				log.Error(ctx, "主动重发现失败", zap.String("serverName", serverName), zap.Error(err))
			}
		})
	}
}
