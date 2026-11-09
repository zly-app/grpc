package app

import "testing"

func TestNewAndOptions(t *testing.T) {
	a := New(WithoutErrCodeHook())
	if a == nil {
		t.Fatal("组合根为空")
	}
	if got := len(a.Options()); got != 2 {
		t.Fatalf("默认选项应只有装配/关闭两个 handler, 实际 %d", got)
	}
	if a.Server("hello") == nil {
		t.Fatal("服务声明器为空")
	}
	if a.ClientConn("hello") == nil {
		t.Fatal("客户端连接为空")
	}
}

func TestEnabledServicesAddOptions(t *testing.T) {
	a := New(WithServer(), WithGateway(), WithoutErrCodeHook())
	if got := len(a.Options()); got != 4 {
		t.Fatalf("启用两个服务后应有 4 个选项, 实际 %d", got)
	}
}

func TestDuplicateServerPanics(t *testing.T) {
	a := New(WithoutErrCodeHook())
	_ = a.Server("dup")
	defer func() {
		if recover() == nil {
			t.Fatal("同名服务重复声明应 panic")
		}
	}()
	_ = a.Server("dup")
}

func TestCloseIsIdempotent(t *testing.T) {
	a := New(WithoutErrCodeHook())
	a.Close()
	a.Close()
}
