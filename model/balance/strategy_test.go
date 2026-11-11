package balance

import (
	"fmt"
	"testing"
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
	if loadOf(a) != 50 {
		t.Fatalf("weight=100 inflight=5 的负载 = %d, want 50", loadOf(a))
	}
	if loadOf(b) != 100 {
		t.Fatalf("weight=50 inflight=5 的负载 = %d, want 100", loadOf(b))
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
