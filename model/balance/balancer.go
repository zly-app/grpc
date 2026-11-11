package balance

import (
	"sort"
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/foundation/addr"
)

var logger = grpclog.Component("zly-balance")

// builder 是策略名到 balancer 的注册项。
//
// 自己实现 balancer.Balancer 而不是用 balancer/base：base 每次子连接状态变化都
// 重建 PickerBuilder，而策略里带着轮询计数、hash 环和在途请求数，这些必须活过重建。
type builder struct {
	name        string
	newStrategy func() strategy
}

func (b *builder) Name() string { return b.name }

func (b *builder) Build(cc balancer.ClientConn, _ balancer.BuildOptions) balancer.Balancer {
	cb := &connBalancer{
		cc:       cc,
		strategy: b.newStrategy(),
		conns:    make(map[string]*connState),
		live:     make(map[balancer.SubConn]*connState),
		state:    connectivity.Connecting,
	}
	cb.picker = base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	return cb
}

// connState 是一条子连接的状态；inst 是稳定对象，随连接一起复用，
// 承载跨 picker 重建必须保留的东西（名字、权重、在途请求数）。
type connState struct {
	sc    balancer.SubConn
	inst  *instance
	state connectivity.State
}

type connBalancer struct {
	cc       balancer.ClientConn
	strategy strategy

	mu sync.Mutex
	// conns 是当前 resolver 地址集（Address.Addr → 连接），用于和上一次地址集做差。
	conns map[string]*connState
	// live 是本 ClientConn 上所有尚未 Shutdown 的子连接，含已从地址集移除、
	// 正在等 Shutdown 回调的那些。聚合状态只能按 live 计，否则计数会泄漏。
	live map[balancer.SubConn]*connState

	agg    balancer.ConnectivityStateEvaluator
	state  connectivity.State
	picker balancer.Picker
	last   []*instance // 上一次交给策略的实例集，用于跳过无意义的环重建
}

// UpdateClientConnState 把 resolver 的地址集同步成子连接集合。
//
// NewSubConn/Connect/Shutdown 都可能同步回调子连接状态，所以一律放到锁外。
func (b *connBalancer) UpdateClientConnState(s balancer.ClientConnState) error {
	addrs := s.ResolverState.Addresses
	if len(addrs) == 0 {
		b.dropAll()
		b.ResolverError(errNoInstance)
		return balancer.ErrBadResolverState
	}

	var (
		toConnect []balancer.SubConn
		toShut    []balancer.SubConn
	)

	b.mu.Lock()
	seen := make(map[string]struct{}, len(addrs))
	for _, a := range addrs {
		seen[a.Addr] = struct{}{}
		if cs, ok := b.conns[a.Addr]; ok {
			b.applyAddr(cs, a) // 同一地址的权重/名字可能被重新注册
			continue
		}

		// 闭包捕获 sc 本体（而非地址串）：地址可能同时被移除，
		// 只有 SubConn 能唯一标识一条连接。StateListener 不会在 Connect 之前触发。
		var sc balancer.SubConn
		var err error
		opts := balancer.NewSubConnOptions{
			StateListener: func(st balancer.SubConnState) { b.onSubConnState(sc, st) },
		}
		sc, err = b.cc.NewSubConn([]resolver.Address{a}, opts)
		if err != nil {
			logger.Warningf("创建子连接失败: %v", err)
			continue
		}

		cs := &connState{sc: sc, inst: &instance{id: a.Addr, sc: sc}, state: connectivity.Idle}
		b.applyAddr(cs, a)
		b.conns[a.Addr] = cs
		b.live[sc] = cs
		b.agg.RecordTransition(connectivity.Shutdown, connectivity.Idle)
		toConnect = append(toConnect, sc)
	}
	for key, cs := range b.conns {
		if _, ok := seen[key]; !ok {
			delete(b.conns, key)
			toShut = append(toShut, cs.sc) // live 里的记录等 Shutdown 回调清理
		}
	}

	b.rebuildLocked()
	out := balancer.State{ConnectivityState: b.state, Picker: b.picker}
	b.mu.Unlock()

	for _, sc := range toConnect {
		sc.Connect()
	}
	for _, sc := range toShut {
		sc.Shutdown()
	}
	b.cc.UpdateState(out)
	return nil
}

// dropAll 清空地址集：留下的 live 记录等各自的 Shutdown 回调回收。
func (b *connBalancer) dropAll() {
	b.mu.Lock()
	toShut := make([]balancer.SubConn, 0, len(b.conns))
	for key, cs := range b.conns {
		delete(b.conns, key)
		toShut = append(toShut, cs.sc)
	}
	b.rebuildLocked()
	b.mu.Unlock()

	for _, sc := range toShut {
		sc.Shutdown()
	}
}

func (b *connBalancer) applyAddr(cs *connState, a resolver.Address) {
	info := addr.Get(a)
	name := info.Name
	if name == "" {
		name = a.Addr // 属性缺失时用线缆地址兜底，一致 hash 环不接受空名字
	}
	weight := info.Weight
	if weight == 0 {
		weight = addr.DefWeight // 0 在加权策略里等于「摘掉该实例」
	}
	cs.inst.id = a.Addr
	cs.inst.name = name
	cs.inst.weight = weight
}

func (b *connBalancer) onSubConnState(sc balancer.SubConn, st balancer.SubConnState) {
	b.mu.Lock()
	cs := b.live[sc]
	if cs == nil {
		b.mu.Unlock()
		return
	}

	old := cs.state
	if old == connectivity.TransientFailure &&
		(st.ConnectivityState == connectivity.Connecting || st.ConnectivityState == connectivity.Idle) {
		// 进入 TF 之后，Connecting/Idle 只当重连噪声，否则多个后端同时不可用
		// 会让聚合状态一直停在 CONNECTING。
		b.mu.Unlock()
		if st.ConnectivityState == connectivity.Idle {
			sc.Connect()
		}
		return
	}

	cs.state = st.ConnectivityState
	if cs.state == connectivity.Shutdown {
		delete(b.live, sc)
	}

	b.state = b.agg.RecordTransition(old, cs.state)
	b.rebuildLocked()
	out := balancer.State{ConnectivityState: b.state, Picker: b.picker}
	b.mu.Unlock()

	if cs.state == connectivity.Idle {
		sc.Connect()
	}
	b.cc.UpdateState(out)
}

// rebuildLocked 按「地址集内且 Ready」重建 picker；实例集没变时不动策略（不重建 hash 环）。
func (b *connBalancer) rebuildLocked() {
	ready := make([]*instance, 0, len(b.conns))
	for _, cs := range b.live {
		if cs.state != connectivity.Ready || cs.inst == nil {
			continue
		}
		// 地址已被移除、正在等 Shutdown 的连接不再参与选点
		if _, ok := b.conns[cs.inst.id]; !ok {
			continue
		}
		ready = append(ready, cs.inst)
	}

	if len(ready) == 0 {
		b.picker = base.NewErrPicker(balancer.ErrNoSubConnAvailable)
		b.last = nil
		return
	}

	// 排序让「实例集是否变化」可比较，也让环/线段的构建顺序确定。
	sort.Slice(ready, func(i, j int) bool {
		if ready[i].name != ready[j].name {
			return ready[i].name < ready[j].name
		}
		return ready[i].id < ready[j].id
	})

	if !sameInstances(b.last, ready) {
		b.strategy.Update(ready)
		b.last = ready
	}

	byName := make(map[string]*instance, len(ready))
	for _, inst := range ready {
		if _, ok := byName[inst.name]; !ok {
			byName[inst.name] = inst
		}
	}
	b.picker = &picker{strategy: b.strategy, byName: byName}
}

func sameInstances(a, b []*instance) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] || a[i].name != b[i].name || a[i].weight != b[i].weight {
			return false
		}
	}
	return true
}

func (b *connBalancer) ResolverError(err error) {
	b.mu.Lock()
	if len(b.conns) == 0 {
		b.state = connectivity.TransientFailure
		b.picker = base.NewErrPicker(err)
	}
	out := balancer.State{ConnectivityState: b.state, Picker: b.picker}
	b.mu.Unlock()
	b.cc.UpdateState(out)
}

// UpdateSubConnState 不会被调用：状态一律经 StateListener 回调。
func (b *connBalancer) UpdateSubConnState(sc balancer.SubConn, state balancer.SubConnState) {
	logger.Errorf("UpdateSubConnState(%v, %+v) 被意外调用", sc, state)
}

// Close 不主动关闭子连接：ClientConn 关闭时统一处理。
func (b *connBalancer) Close() {}

// ExitIdle 不做处理：子连接始终由 UpdateClientConnState 主动 Connect。
func (b *connBalancer) ExitIdle() {}
