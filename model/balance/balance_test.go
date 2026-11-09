package balance

import "testing"

func TestSpecTableIsComplete(t *testing.T) {
	for _, name := range []string{RoundRobin, WeightRandom, WeightHash, WeightConsistentHash} {
		if _, ok := specs()[name]; !ok {
			t.Fatalf("策略表缺少 %s", name)
		}
	}
	if specs()[RoundRobin].useHashKey || !specs()[WeightConsistentHash].useHashKey {
		t.Fatal("hashKey 使用标记错误")
	}
}

func TestDialOption(t *testing.T) {
	tab := New()
	if _, err := tab.DialOption(WeightConsistentHash); err != nil {
		t.Fatalf("合法策略取 dial option 失败: %v", err)
	}
	if _, err := tab.DialOption("ROUND_ROBIN"); err != nil {
		t.Fatalf("大小写归一失败: %v", err)
	}
	if _, err := tab.DialOption("not_exist"); err == nil {
		t.Fatal("未知策略应显式报错")
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	tab := New()
	tab.Install()
	tab.Install() // 不得因重复注册 gRPC 全局 balancer 而 panic
}
