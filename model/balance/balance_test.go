package balance

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"

	"github.com/zly-app/grpc/foundation/addr"
	"github.com/zly-app/grpc/foundation/ctxkey"
)

// fakeSubConn 内嵌真实接口以满足 gRPC 强制的嵌入约束（internal 包外部不可 import），
// 嵌入值为 nil，测试里只调用自己实现的方法。
type fakeSubConn struct {
	balancer.SubConn
	addr     string
	listener func(balancer.SubConnState)
	connects int
	shutdown bool
}

func (f *fakeSubConn) UpdateAddresses([]resolver.Address) {}

func (f *fakeSubConn) Connect() { f.connects++ }

func (f *fakeSubConn) GetOrBuildProducer(balancer.ProducerBuilder) (balancer.Producer, func()) {
	return nil, func() {}
}

func (f *fakeSubConn) Shutdown() { f.shutdown = true }

func (f *fakeSubConn) RegisterHealthListener(func(balancer.SubConnState)) {}

func (f *fakeSubConn) setState(s connectivity.State) {
	if f.listener != nil {
		f.listener(balancer.SubConnState{ConnectivityState: s})
	}
}

// fakeClientConn 只实现状态机用得到的方法，其余靠内嵌 nil 接口满足编译。
type fakeClientConn struct {
	balancer.ClientConn

	mu    sync.Mutex
	subs  []*fakeSubConn
	state balancer.State
}

func (f *fakeClientConn) NewSubConn(addrs []resolver.Address, opts balancer.NewSubConnOptions) (balancer.SubConn, error) {
	sc := &fakeSubConn{addr: addrs[0].Addr, listener: opts.StateListener}
	f.mu.Lock()
	f.subs = append(f.subs, sc)
	f.mu.Unlock()
	return sc, nil
}

func (f *fakeClientConn) UpdateState(s balancer.State) {
	f.mu.Lock()
	f.state = s
	f.mu.Unlock()
}

func (f *fakeClientConn) snapshot() ([]*fakeSubConn, balancer.State) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*fakeSubConn(nil), f.subs...), f.state
}

func newFakeInst(id, name string, weight uint16) *instance {
	return &instance{id: id, name: name, weight: weight, sc: &fakeSubConn{addr: id}}
}

func TestSpecTableIsComplete(t *testing.T) {
	for _, name := range []string{RoundRobin, WeightRandom, WeightHash, WeightConsistentHash, LeastRequest} {
		s, ok := specs()[name]
		if !ok {
			t.Fatalf("策略表缺少 %s", name)
		}
		if s.newStrategy() == nil {
			t.Fatalf("策略 %s 未构造出实现", name)
		}
	}
	if Default != LeastRequest {
		t.Fatalf("默认策略 = %s, want %s", Default, LeastRequest)
	}
}

// 策略名不能被 grpc-go 的内置均衡器占用：注册表是进程级全局表，同名会静默覆盖别人。
func TestStrategyNamesAreNamespaced(t *testing.T) {
	for name := range specs() {
		if !strings.HasPrefix(name, "zly_") {
			t.Fatalf("策略名 %q 缺少 zly_ 前缀，可能与 gRPC 内置均衡器撞名", name)
		}
	}
	if balancer.Get("round_robin") == nil {
		t.Fatal("内置 round_robin 应仍由 grpc-go 提供")
	}
}

func TestDialOption(t *testing.T) {
	tab := New()
	if _, err := tab.DialOption(WeightConsistentHash); err != nil {
		t.Fatalf("合法策略取 dial option 失败: %v", err)
	}
	if _, err := tab.DialOption("ZLY_ROUND_ROBIN"); err != nil {
		t.Fatalf("大小写归一失败: %v", err)
	}
	if _, err := tab.DialOption("not_exist"); err == nil {
		t.Fatal("未知策略应显式报错")
	}
}

// v0.7.0 之前的策略名要报错，并且报错里必须给出新名字——
// 静默映射会让「我想要一致性 hash」和「我拿到一致性 hash」脱钩。
func TestDialOptionLegacyNameHintsTarget(t *testing.T) {
	tab := New()
	for old, want := range map[string]string{
		"round_robin":            RoundRobin,
		"weight_random":          WeightRandom,
		"weight_hash":            WeightHash,
		"weight_consistent_hash": WeightConsistentHash,
	} {
		_, err := tab.DialOption(old)
		if err == nil {
			t.Fatalf("%q 应报错（已改名）", old)
		}
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("%q 的报错里应包含新名 %q, 实际: %v", old, want, err)
		}
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	tab := New()
	tab.Install()
	tab.Install() // 不得因重复注册 gRPC 全局 balancer 而 panic
	for _, name := range []string{RoundRobin, WeightRandom, WeightHash, WeightConsistentHash, LeastRequest} {
		if balancer.Get(name) == nil {
			t.Fatalf("策略 %q 未注册进 gRPC", name)
		}
	}
}

// ---------- picker ----------

func readyPicker(t *testing.T, st strategy, insts ...*instance) *picker {
	t.Helper()
	st.Update(insts)
	byName := make(map[string]*instance, len(insts))
	for _, i := range insts {
		byName[i.name] = i
	}
	return &picker{strategy: st, byName: byName}
}

func TestPickByTarget(t *testing.T) {
	a := newFakeInst("10.0.0.1:1", "node-a", 100)
	b := newFakeInst("10.0.0.2:1", "node-b", 100)
	p := readyPicker(t, new(roundStrategy), a, b)

	ctx := contextWithTarget("node-b")
	res, err := p.Pick(balancer.PickInfo{Ctx: ctx})
	if err != nil {
		t.Fatalf("点名实例失败: %v", err)
	}
	if res.SubConn != b.sc {
		t.Fatal("未选中目标实例")
	}
}

// 点名实例不在 Ready 集合时必须明确失败，不能退化成随便选一个。
func TestPickTargetUnavailable(t *testing.T) {
	a := newFakeInst("10.0.0.1:1", "node-a", 100)
	p := readyPicker(t, new(roundStrategy), a)

	_, err := p.Pick(balancer.PickInfo{Ctx: contextWithTarget("node-x")})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unavailable {
		t.Fatalf("应回 Unavailable, 实际 %v", err)
	}
}

// Done 回调必须回收在途计数，否则 P2C 的负载会只增不减。
func TestPickDoneReleasesInflight(t *testing.T) {
	a := newFakeInst("10.0.0.1:1", "node-a", 100)
	p := readyPicker(t, new(leastRequestStrategy), a)

	res, err := p.Pick(balancer.PickInfo{Ctx: context.Background()})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.inflight.Load(); got != 1 {
		t.Fatalf("选中后在途数 = %d, want 1", got)
	}
	if res.Done == nil {
		t.Fatal("PickResult 必须挂 Done 回调")
	}
	res.Done(balancer.DoneInfo{})
	if got := a.inflight.Load(); got != 0 {
		t.Fatalf("Done 后在途数 = %d, want 0", got)
	}
}

func TestPickErrorSemantics(t *testing.T) {
	if err := pickError(errNoInstance); err != balancer.ErrNoSubConnAvailable {
		t.Fatalf("无实例应等待新 picker, 实际 %v", err)
	}
	boom := fmt.Errorf("boom")
	if got := pickError(boom); got != boom {
		t.Fatalf("其它错误应原样返回, 实际 %v", got)
	}
}

// ---------- balancer 状态机 ----------

func TestBalancerLifecycle(t *testing.T) {
	cc := new(fakeClientConn)
	b := (&builder{name: RoundRobin, newStrategy: func() strategy { return new(roundStrategy) }}).Build(cc, balancer.BuildOptions{})

	err := b.UpdateClientConnState(balancer.ClientConnState{
		ResolverState: resolver.State{Addresses: []resolver.Address{
			addr.Set(resolver.Address{Addr: "10.0.0.1:3000"}, &addr.Info{Name: "node-a", Weight: 100}),
			addr.Set(resolver.Address{Addr: "10.0.0.2:3000"}, &addr.Info{Name: "node-b", Weight: 100}),
		}},
	})
	if err != nil {
		t.Fatalf("同步地址失败: %v", err)
	}

	subs, _ := cc.snapshot()
	if len(subs) != 2 {
		t.Fatalf("子连接数 = %d, want 2", len(subs))
	}
	for _, sc := range subs {
		if sc.connects != 1 {
			t.Fatalf("子连接 %s 应被 Connect 一次, 实际 %d", sc.addr, sc.connects)
		}
	}

	// 未 Ready 时 picker 不可用
	if _, err := cc.state.Picker.Pick(balancer.PickInfo{}); err == nil {
		t.Fatal("没有 Ready 子连接时不应选出实例")
	}

	subs[0].setState(connectivity.Ready)
	if got := cc.state.ConnectivityState; got != connectivity.Ready {
		t.Fatalf("聚合状态 = %v, want READY", got)
	}
	res, err := cc.state.Picker.Pick(balancer.PickInfo{})
	if err != nil {
		t.Fatalf("Ready 后选点失败: %v", err)
	}
	if res.SubConn != subs[0] {
		t.Fatal("选中的不是唯一可用的子连接")
	}

	// 地址移除后子连接必须被 shutdown
	if err := b.UpdateClientConnState(balancer.ClientConnState{
		ResolverState: resolver.State{Addresses: []resolver.Address{
			addr.Set(resolver.Address{Addr: "10.0.0.2:3000"}, &addr.Info{Name: "node-b", Weight: 100}),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if !subs[0].shutdown {
		t.Fatal("被移除的地址应 shutdown 子连接")
	}
}

// 权重为 0 / 名字缺失时不能 panic，也不能把地址静默摘掉。
func TestBalancerFallsBackOnMissingAttributes(t *testing.T) {
	for _, name := range []string{RoundRobin, WeightRandom, WeightHash, WeightConsistentHash, LeastRequest} {
		t.Run(name, func(t *testing.T) {
			cc := new(fakeClientConn)
			b := (&builder{name: name, newStrategy: specs()[name].newStrategy}).Build(cc, balancer.BuildOptions{})

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("缺属性时不应 panic: %v", r)
				}
			}()

			if err := b.UpdateClientConnState(balancer.ClientConnState{
				ResolverState: resolver.State{Addresses: []resolver.Address{{Addr: "10.0.0.9:3000"}}},
			}); err != nil {
				t.Fatal(err)
			}
			subs, _ := cc.snapshot()
			subs[0].setState(connectivity.Ready)

			if _, err := cc.state.Picker.Pick(balancer.PickInfo{}); err != nil {
				t.Fatalf("缺属性时应回落到默认权重继续选点: %v", err)
			}
		})
	}
}

func contextWithTarget(target string) context.Context {
	return ctxkey.InjectTarget(context.Background(), target)
}
