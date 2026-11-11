package discover

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	rredis "github.com/redis/go-redis/v9"
	"github.com/zly-app/component/redis"
	"github.com/zly-app/zapp/log"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"
	gresolver "google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/foundation/addr"
	fresolver "github.com/zly-app/grpc/foundation/resolver"
	"github.com/zly-app/grpc/model/endpoint"
)

// ErrNoInstance 表示当前没有可用实例；地址可能随后出现，调用方可按需重试。
var ErrNoInstance = errors.New("服务没有可用实例")

const (
	// fetchTimeout 限制一次 HGETALL 的耗时。
	fetchTimeout = 3 * time.Second
	// confirmTimeout 限制一次「下线信号二次确认」的耗时。
	confirmTimeout = time.Second
	// emptyRefreshInterval 限制「地址为空时补拉」的频率，避免调用方重试变成对 Redis 的压测。
	emptyRefreshInterval = time.Second
)

// RedisDiscover 拥有每个服务的地址状态机；订阅信号与定时重发现是两条互补的更新路径，
// 都只推状态、不触发新的发现请求（环的终止保障）。
type RedisDiscover struct {
	client redis.UniversalClient
	parent context.Context
	sub    *rredis.PubSub
	ticker *time.Ticker
	stop   chan struct{}
	wg     sync.WaitGroup

	stopOnce sync.Once
	mx       sync.Mutex
	res      map[string]*entry
}

// entry 是单个服务的地址状态机：写者串行（mu），读者无锁（view 是不可变快照）。
//
// ver 每次状态变更 +1，用来仲裁「信号」与「全量快照」的先后：
// HGETALL 与信号应用之间没有因果序，快照可能比信号旧，
// 若直接覆盖就会把刚注册的实例摘掉（要等下一轮重发现才恢复）。
type entry struct {
	b    *fresolver.Builder
	view atomic.Pointer[entryView]

	mu     sync.Mutex
	ver    uint64
	seen   time.Time // 最后一次状态确认（信号或快照），用于空态补拉节流
	pulled time.Time // 最后一次成功全量拉取，用于重发现节流
}

// entryView 是不可变快照：records 一旦发布就不再被修改。
type entryView struct {
	records []*endpoint.Record
}

func newEntry() *entry {
	e := &entry{b: fresolver.NewBuilderWithScheme(endpoint.TypeRedis)}
	e.view.Store(&entryView{records: nil})
	return e
}

func (e *entry) records() []*endpoint.Record { return e.view.Load().records }

func (e *entry) empty() bool { return len(e.view.Load().records) == 0 }

func (e *entry) version() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ver
}

func (e *entry) lastSeen() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.seen
}

func (e *entry) lastPull() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pulled
}

func (e *entry) markSeen() {
	e.mu.Lock()
	e.seen = time.Now()
	e.mu.Unlock()
}

// publish 发布新状态并广播给已连接的 ClientConn；调用方必须持有 e.mu。
func (e *entry) publish(records []*endpoint.Record) {
	e.ver++
	e.seen = time.Now()
	e.view.Store(&entryView{records: records})
	e.b.UpdateState(gresolver.State{Addresses: makeAddress(records)})
}

// upsert 应用一条注册信号：同 endpoint 以最新一次为准。
func (e *entry) upsert(rec *endpoint.Record) {
	if rec == nil || rec.Endpoint == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	cur := e.view.Load().records
	next := make([]*endpoint.Record, 0, len(cur)+1)
	replaced := false
	for _, v := range cur {
		if v.Endpoint == rec.Endpoint {
			next = append(next, rec)
			replaced = true
			continue
		}
		next = append(next, v)
	}

	if replaced && sameRecords(next, cur) {
		e.seen = time.Now() // 状态没变也要刷新确认时间
		return
	}
	if !replaced {
		next = append(next, rec)
		sortRecords(next)
	}
	e.publish(next)
}

// drop 应用一条下线信号。
func (e *entry) drop(ep string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cur := e.view.Load().records
	next := make([]*endpoint.Record, 0, len(cur))
	for _, v := range cur {
		if v.Endpoint != ep {
			next = append(next, v)
		}
	}
	if len(next) == len(cur) {
		e.seen = time.Now()
		return
	}
	e.publish(next)
}

// merge 应用一次全量快照。
//
// startVer 是发起拉取前读到的版本：若期间有信号推进过状态，说明快照可能比信号旧，
// 此时只补不删（快照里有、本地没有的补上；本地有、快照没有的保留）；
// 否则快照是权威，允许删除——TTL 过期只能靠全量发现感知。
func (e *entry) merge(records []*endpoint.Record, startVer uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cur := e.view.Load().records
	next := records
	if e.ver != startVer {
		next = unionRecords(cur, records)
	}
	sortRecords(next)

	now := time.Now()
	e.pulled = now // 与状态是否变化无关：这是一次成功的全量确认
	if sameRecords(next, cur) {
		e.seen = now
		return
	}
	e.publish(next)
}

// unionRecords 以 cur 为准合并快照：冲突时保留本地（信号比快照新）。
func unionRecords(cur, snapshot []*endpoint.Record) []*endpoint.Record {
	seen := make(map[string]struct{}, len(cur)+len(snapshot))
	next := make([]*endpoint.Record, 0, len(cur)+len(snapshot))
	for _, list := range [][]*endpoint.Record{cur, snapshot} {
		for _, v := range list {
			if _, ok := seen[v.Endpoint]; ok {
				continue
			}
			seen[v.Endpoint] = struct{}{}
			next = append(next, v)
		}
	}
	return next
}

// sameRecords 判断两次状态是否等价；两边的顺序都必须已经按 endpoint 固定。
func sameRecords(a, b []*endpoint.Record) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Endpoint != b[i].Endpoint || a[i].Name != b[i].Name || a[i].Weight != b[i].Weight {
			return false
		}
	}
	return true
}

// sortRecords 按 endpoint 排序：地址集合稳定，避免无变化的发现结果触发 resolver 更新。
func sortRecords(list []*endpoint.Record) {
	sort.Slice(list, func(i, j int) bool { return list[i].Endpoint < list[j].Endpoint })
}

// NewRedisDiscover 建发现器并启动订阅与重发现循环；ctx 作为后台任务的父上下文。
func NewRedisDiscover(ctx context.Context, address string, creator redis.Creator) (*RedisDiscover, error) {
	client, err := creator.GetClient(address)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r := &RedisDiscover{
		client: client,
		parent: ctx,
		sub:    client.Subscribe(ctx),
		ticker: time.NewTicker(endpoint.ReDiscoverInterval * time.Second),
		stop:   make(chan struct{}),
		res:    make(map[string]*entry),
	}
	r.wg.Add(2)
	go r.consumeSignals()
	go r.reDiscoverLoop()
	return r, nil
}

// Builder 返回该服务的 resolver.Builder。
//
// 顺序很关键：先登记 entry → 再订阅信号 → 最后拉初始快照。
// 反过来的话，「快照之后、订阅之前」发生的注册会永久丢失（只能等 30s 重发现）。
func (r *RedisDiscover) Builder(ctx context.Context, serverName string) (gresolver.Builder, error) {
	e, err := r.ensureEntry(ctx, serverName)
	if err != nil {
		return nil, err
	}

	if e.empty() {
		r.refresh(ctx, serverName, e)
	}
	if e.empty() {
		return nil, fmt.Errorf("grpc 服务发现失败, %w: %s", ErrNoInstance, serverName)
	}
	return e.b, nil
}

// ensureEntry 取（或首次建立）服务的状态机：登记 → 订阅 → 拉快照。
func (r *RedisDiscover) ensureEntry(ctx context.Context, serverName string) (*entry, error) {
	r.mx.Lock()
	if e, ok := r.res[serverName]; ok {
		r.mx.Unlock()
		return e, nil
	}
	e := newEntry()
	r.res[serverName] = e // 先登记：信号处理只认已登记的服务
	r.mx.Unlock()

	if err := r.sub.Subscribe(ctx, endpoint.SignalKey(serverName)); err != nil {
		// 订阅是加速手段，不是可用性前提：reDiscoverLoop 每 ReDiscoverInterval 会拉一次全量，
		// 订阅不可用只意味着「注册变更最多延迟一个周期」。
		//
		// 真机验证暴露过：某些 Redis 代理允许 HSET、却会定期掐断空闲的 PubSub 长连接。
		// 如果这里直接失败，那类 Redis 上客户端会完全不可用——而地址其实一直在；
		// 反过来先拉快照成功，客户端至少能用最后一次确认的地址集。
		log.Error(ctx, "订阅注册信号失败（降级为周期重发现）",
			zap.String("serverName", serverName), zap.Error(err))
	}

	// 版本必须在拉取之前取：拉取期间到达的信号会让版本增长，merge 据此判断快照是否已过时。
	startVer := e.version()
	records, err := r.fetch(ctx, serverName)
	if err != nil {
		// 拉取失败不阻塞：订阅已生效，信号与后续重发现会补齐。
		log.Error(ctx, "读取注册地址失败", zap.String("serverName", serverName), zap.Error(err))
		e.markSeen()
		return e, nil
	}
	e.merge(records, startVer)
	return e, nil
}

// refresh 在地址为空时补拉一次，让「客户端先起、服务后注册」能立刻恢复而不是等 30s。
func (r *RedisDiscover) refresh(ctx context.Context, serverName string, e *entry) {
	if time.Since(e.lastSeen()) < emptyRefreshInterval {
		return
	}
	startVer := e.version()
	records, err := r.fetch(ctx, serverName)
	if err != nil {
		e.markSeen()
		return
	}
	e.merge(records, startVer)
}

// Close 关订阅与重发现循环，并等后台协程退出；可重复调用。
func (r *RedisDiscover) Close() {
	_ = r.sub.Close()
	r.ticker.Stop()
	r.stopOnce.Do(func() { close(r.stop) })
	r.wg.Wait()
}

func (r *RedisDiscover) entryOf(serverName string) *entry {
	r.mx.Lock()
	defer r.mx.Unlock()
	return r.res[serverName]
}

func (r *RedisDiscover) consumeSignals() {
	defer r.wg.Done()
	for msg := range r.sub.Channel() {
		if !strings.HasPrefix(msg.Channel, endpoint.KeySignalPrefix) {
			continue
		}
		serverName := msg.Channel[len(endpoint.KeySignalPrefix):]

		e := r.entryOf(serverName)
		if e == nil {
			continue // 未登记的服务：没有订阅者，直接丢弃
		}

		sig := endpoint.Signal{}
		if err := sonic.UnmarshalString(msg.Payload, &sig); err != nil || sig.Record == nil {
			continue // 异常信号显式丢弃，不推进状态
		}
		if sig.IsUnReg {
			// 下线信号必须二次确认：信号可能乱序或重放，而「字段仍在」说明实例其实活着，
			// 误删会造成无谓的流量中断，且没有信号可以把它加回来。
			if r.gone(serverName, sig.Record.Endpoint) {
				e.drop(sig.Record.Endpoint)
			}
			continue
		}
		e.upsert(sig.Record)
	}
}

// gone 确认注册字段确实已不存在，才允许应用下线信号。
func (r *RedisDiscover) gone(serverName, ep string) bool {
	ctx, cancel := context.WithTimeout(r.parent, confirmTimeout)
	defer cancel()

	ok, err := r.client.HExists(ctx, endpoint.RegKey(serverName), ep).Result()
	if err != nil {
		log.Warn(ctx, "下线信号确认失败，本次跳过",
			zap.String("serverName", serverName), zap.String("endpoint", ep), zap.Error(err))
		return false
	}
	return !ok
}

func (r *RedisDiscover) reDiscoverLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ticker.C:
			r.reDiscoverAll()
		case <-r.stop:
			return
		case <-r.parent.Done():
			return
		}
	}
}

// reDiscoverAll 对所有已订阅服务并发重发现一次。
// 与心跳同理：不借用共享任务池，避免被业务任务挤住导致地址长期不刷新。
func (r *RedisDiscover) reDiscoverAll() {
	ctx, span := utils.Trace.StartSpan(r.parent, "ReDiscoverGrpcServer")
	defer utils.Trace.EndSpan(span)

	r.mx.Lock()
	snapshot := make(map[string]*entry, len(r.res))
	for k, v := range r.res {
		snapshot[k] = v
	}
	r.mx.Unlock()

	var wg sync.WaitGroup
	for name, e := range snapshot {
		wg.Add(1)
		go func(name string, e *entry) {
			defer wg.Done()
			r.reDiscoverOne(ctx, name, e)
		}(name, e)
	}
	wg.Wait()
}

func (r *RedisDiscover) reDiscoverOne(ctx context.Context, serverName string, e *entry) {
	// 节流只看「上次成功全量拉取」，不看信号：信号只能证明某实例还在，
	// 无法证明另一个实例的 TTL 没到期，所以过期只能靠定时全量发现感知。
	if time.Since(e.lastPull()) < endpoint.ReDiscoverInterval/2*time.Second {
		return
	}

	startVer := e.version()
	records, err := r.fetch(ctx, serverName)
	if err != nil {
		e.markSeen() // 失败只推进见闻时间，下一轮仍会重试
		return
	}
	e.merge(records, startVer)
}

// fetch 读取一个服务的全部实例：纯只读，过期由 Redis 的字段级 TTL 负责。
// 反序列化失败的记录只跳过，等 TTL 到期自然消失。
func (r *RedisDiscover) fetch(ctx context.Context, serverName string) ([]*endpoint.Record, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	data, err := r.client.HGetAll(ctx, endpoint.RegKey(serverName)).Result()
	if err != nil {
		return nil, err
	}

	out := make([]*endpoint.Record, 0, len(data))
	seen := make(map[string]struct{}, len(data))
	for field, text := range data {
		rec := endpoint.Record{}
		if err := sonic.UnmarshalString(text, &rec); err != nil {
			log.Warn(ctx, "注册记录反序列化失败",
				zap.String("serverName", serverName), zap.String("field", field), zap.Error(err))
			continue
		}
		if rec.Endpoint == "" {
			rec.Endpoint = field
		}
		if _, ok := seen[rec.Endpoint]; ok {
			continue // 同一 endpoint 只保留一条，避免加权均衡下权重翻倍
		}
		seen[rec.Endpoint] = struct{}{}
		out = append(out, &rec)
	}
	sortRecords(out)
	return out, nil
}

func makeAddress(records []*endpoint.Record) []gresolver.Address {
	list := make([]gresolver.Address, 0, len(records))
	for _, a := range records {
		ra := gresolver.Address{Addr: a.Endpoint}
		ra = addr.Set(ra, &addr.Info{Name: a.Name, Endpoint: a.Endpoint, Weight: a.Weight})
		list = append(list, ra)
	}
	return list
}
