package balance

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func mkInsts(n int, weight uint16) []*instance {
	out := make([]*instance, n)
	for i := range out {
		out[i] = newFakeInst(fmt.Sprintf("10.0.0.%d:3000", i+1), fmt.Sprintf("node-%d", i), weight)
	}
	return out
}

func mustPick(t *testing.T, s strategy, key string) *instance {
	t.Helper()
	inst, err := s.Pick(key)
	if err != nil {
		t.Fatalf("选点失败: %v", err)
	}
	return inst
}

func TestNoInstance(t *testing.T) {
	for _, s := range []strategy{
		new(roundStrategy), new(weightRandomStrategy), new(weightHashStrategy),
		new(ringStrategy), new(leastRequestStrategy),
	} {
		s.Update(nil)
		if _, err := s.Pick(""); err == nil {
			t.Fatalf("%T 在无实例时应报错", s)
		}
	}
}

func TestRoundRobinCycles(t *testing.T) {
	insts := mkInsts(3, 100)
	s := new(roundStrategy)
	s.Update(insts)

	count := map[string]int{}
	for i := 0; i < 30; i++ {
		count[mustPick(t, s, "").name]++
	}
	for _, inst := range insts {
		if count[inst.name] != 10 {
			t.Fatalf("30 次轮询分布不均: %v", count)
		}
	}
}

// 轮询位置不能因为实例集刷新而重置：否则每次地址变化都会从第一个实例重新开始。
func TestRoundRobinSurvivesUpdate(t *testing.T) {
	insts := mkInsts(3, 100)
	s := new(roundStrategy)
	s.Update(insts)

	if got := mustPick(t, s, ""); got != insts[0] {
		t.Fatalf("第 1 次应选 %s, 实际 %s", insts[0].name, got.name)
	}
	s.Update(insts) // 模拟地址集刷新（实例未变）
	if got := mustPick(t, s, ""); got != insts[1] {
		t.Fatalf("刷新后应继续轮询到 %s, 实际 %s", insts[1].name, got.name)
	}
}

func TestWeightRandomDistribution(t *testing.T) {
	a := newFakeInst("10.0.0.1:3000", "node-a", 100)
	b := newFakeInst("10.0.0.2:3000", "node-b", 300)

	s := new(weightRandomStrategy)
	s.Update([]*instance{a, b})

	const n = 20000
	hit := 0
	for i := 0; i < n; i++ {
		if mustPick(t, s, "") == a {
			hit++
		}
	}
	ratio := float64(hit) / float64(n)
	if ratio < 0.22 || ratio > 0.28 {
		t.Fatalf("权重 1:3 时低权重实例占比 = %.3f, 期望约 0.25", ratio)
	}
}

func TestWeightHashStable(t *testing.T) {
	insts := mkInsts(5, 100)
	s := new(weightHashStrategy)
	s.Update(insts)

	first := mustPick(t, s, "user-42")
	for i := 0; i < 100; i++ {
		if got := mustPick(t, s, "user-42"); got != first {
			t.Fatalf("同一 key 选点漂移: %s -> %s", first.name, got.name)
		}
	}

	seen := map[string]struct{}{}
	for i := 0; i < 200; i++ {
		seen[mustPick(t, s, fmt.Sprintf("user-%d", i)).name] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("不同 key 应能分散到多个实例, 实际只落 %d 个", len(seen))
	}
}

// 无 hashKey 时降级为轮询，保证流量仍然均匀，而不是全压到一个实例上。
func TestHashStrategiesFallbackToRoundRobin(t *testing.T) {
	for _, s := range []strategy{new(weightHashStrategy), new(ringStrategy)} {
		insts := mkInsts(4, 100)
		s.Update(insts)

		count := map[string]int{}
		for i := 0; i < 40; i++ {
			count[mustPick(t, s, "").name]++
		}
		if len(count) != 4 {
			t.Fatalf("%T 无 key 时应轮询到全部实例, 实际 %v", s, count)
		}
		for name, c := range count {
			if c != 10 {
				t.Fatalf("%T 降级轮询分布不均: %s 命中 %d 次", s, name, c)
			}
		}
	}
}

func TestRingStable(t *testing.T) {
	insts := mkInsts(6, 100)
	s := new(ringStrategy)
	s.Update(insts)

	first := mustPick(t, s, "uid-42")
	for i := 0; i < 100; i++ {
		if got := mustPick(t, s, "uid-42"); got != first {
			t.Fatalf("同一 key 在环上漂移: %s -> %s", first.name, got.name)
		}
	}
}

// ketama 的定义性质：新增一个节点时，只有原本会落到新节点的 key 才会迁移，
// 其余 key 的落点必须完全不动。用总权重取模的实现会在这里大面积失败。
func TestRingOnlyNewNodeTakesOver(t *testing.T) {
	insts := mkInsts(9, 100)
	s := new(ringStrategy)
	s.Update(insts)

	const keys = 800
	before := make(map[string]*instance, keys)
	for i := 0; i < keys; i++ {
		key := fmt.Sprintf("uid-%d", i)
		before[key] = mustPick(t, s, key)
	}

	newNode := newFakeInst("10.0.0.99:3000", "node-new", 100)
	s.Update(append(append([]*instance(nil), insts...), newNode))

	moved := 0
	for key, old := range before {
		got := mustPick(t, s, key)
		if got == old {
			continue
		}
		if got != newNode {
			t.Fatalf("key %s 从 %s 迁到了非新增节点 %s", key, old.name, got.name)
		}
		moved++
	}
	if moved == 0 {
		t.Fatal("新节点一个 key 都没接管，环构建可能未生效")
	}
	// 10 个等权实例，理论上约 1/10 的 key 会迁到新节点
	if ratio := float64(moved) / float64(keys); ratio < 0.05 || ratio > 0.2 {
		t.Fatalf("迁移比例 = %.3f, 期望约 0.10", ratio)
	}
}

// P2C 必须把请求从高负载实例挪走。
func TestLeastRequestPrefersIdle(t *testing.T) {
	busy := newFakeInst("10.0.0.1:3000", "node-busy", 100)
	idle := newFakeInst("10.0.0.2:3000", "node-idle", 100)
	busy.inflight.Store(100)

	s := new(leastRequestStrategy)
	s.Update([]*instance{busy, idle})

	for i := 0; i < 200; i++ {
		if got := mustPick(t, s, ""); got != idle {
			t.Fatalf("应选空闲实例, 实际选了 %s", got.name)
		}
	}
}

// 慢节点即使并发不高也要被降权：P2C 的负载必须含延迟，
// 只比在途数的话「低并发但很慢」的节点永远发现不了。
func TestLeastRequestAvoidsSlowInstance(t *testing.T) {
	fast := newFakeInst("10.0.0.1:3000", "node-fast", 100)
	slow := newFakeInst("10.0.0.2:3000", "node-slow", 100)

	// 两者在途数都为 0，只有延迟不同
	for i := 0; i < 10; i++ {
		fast.observe(time.Millisecond, nil)
		slow.observe(50*time.Millisecond, nil)
	}

	s := new(leastRequestStrategy)
	s.Update([]*instance{fast, slow})

	for i := 0; i < 200; i++ {
		if got := mustPick(t, s, ""); got != fast {
			t.Fatalf("应避开慢实例, 实际选了 %s", got.name)
		}
	}
}

// 延迟样本要能过期：节点被持续降权就再也拿不到新样本，
// 旧的高延迟会永久保留、把它饿死。
func TestLeastRequestSampleExpiry(t *testing.T) {
	a := newFakeInst("10.0.0.1:3000", "node-a", 100)
	b := newFakeInst("10.0.0.2:3000", "node-b", 100)

	for i := 0; i < 10; i++ {
		a.observe(time.Millisecond, nil)
		b.observe(50*time.Millisecond, nil)
	}
	now := time.Now().UnixNano()
	if b.latency(now) == 0 {
		t.Fatal("刚采样过的实例不该被判为无样本")
	}

	// 把 b 的采样时刻推到保鲜期之外
	b.sampled.Store(time.Now().Add(-sampleTTL - time.Second).UnixNano())
	if b.latency(time.Now().UnixNano()) != 0 {
		t.Fatal("过期样本应返回 0，让实例回到「未知」被重新采样")
	}

	s := new(leastRequestStrategy)
	s.Update([]*instance{a, b})

	// a 仍有新鲜样本（loadOf > 0），b 未知（loadOf == 0）→ 优先选 b 采样
	if got := mustPick(t, s, ""); got != b {
		t.Fatalf("样本过期后应优先重新采样, 实际选了 %s", got.name)
	}
}

// 失败请求不计入延迟：错误通常在连接/协议层快速返回，
// 计入会把故障节点的延迟拉低，反而让它显得更快。
func TestLeastRequestIgnoresFailedSamples(t *testing.T) {
	inst := newFakeInst("10.0.0.1:3000", "node-a", 100)
	inst.observe(50*time.Millisecond, nil)
	before := inst.ewma.Load()

	inst.observe(time.Millisecond, errors.New("boom"))
	if got := inst.ewma.Load(); got != before {
		t.Fatalf("失败请求不应改动延迟样本: %d -> %d", before, got)
	}
}

// 低并发时所有实例的在途数都是 0，负载相等；此时必须按权重加权随机。
// 只比 in-flight 的话 PublishWeight 在低 QPS 场景完全失效——真机实测权重 200:100
// 串行请求会退化成 1.03:1。
func TestLeastRequestWeightWhenIdle(t *testing.T) {
	small := newFakeInst("10.0.0.1:3000", "node-small", 100)
	big := newFakeInst("10.0.0.2:3000", "node-big", 300)

	s := new(leastRequestStrategy)
	s.Update([]*instance{small, big})

	const n = 8000
	hit := 0
	for i := 0; i < n; i++ {
		if mustPick(t, s, "") == small {
			hit++
		}
	}
	ratio := float64(hit) / float64(n)
	if ratio < 0.20 || ratio > 0.30 {
		t.Fatalf("在途数为 0 时权重 1:3 的低权重实例占比 = %.3f, 期望约 0.25", ratio)
	}
}

// 同等在途数时，权重高的实例负载更低，应被优先选中。
func TestLeastRequestHonorsWeight(t *testing.T) {
	small := newFakeInst("10.0.0.1:3000", "node-small", 100)
	big := newFakeInst("10.0.0.2:3000", "node-big", 400)
	small.inflight.Store(10)
	big.inflight.Store(10)

	s := new(leastRequestStrategy)
	s.Update([]*instance{small, big})

	for i := 0; i < 100; i++ {
		if got := mustPick(t, s, ""); got != big {
			t.Fatalf("应优先选高权重实例, 实际选了 %s", got.name)
		}
	}
}

func TestLoadOfIsWeightAware(t *testing.T) {
	a := newFakeInst("a", "a", 100)
	a.inflight.Store(5)
	b := newFakeInst("b", "b", 50)
	b.inflight.Store(5)
	now := time.Now().UnixNano()
	if loadOf(a, now) != 50 {
		t.Fatalf("weight=100 inflight=5 的负载 = %d, want 50", loadOf(a, now))
	}
	if loadOf(b, now) != 100 {
		t.Fatalf("weight=50 inflight=5 的负载 = %d, want 100", loadOf(b, now))
	}
}

// 权重为 0 的实例在策略层必须回落到默认权重，不能被静默摘掉。
func TestWeightZeroFallsBack(t *testing.T) {
	zero := newFakeInst("10.0.0.1:3000", "node-zero", 0)
	s := new(weightRandomStrategy)
	s.Update([]*instance{zero})

	if got := mustPick(t, s, ""); got != zero {
		t.Fatal("权重为 0 的唯一实例应仍可被选中")
	}
}
