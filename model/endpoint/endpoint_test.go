package endpoint

import (
	"reflect"
	"testing"
)

func TestParseSource(t *testing.T) {
	ok := []struct {
		raw   string
		typ   string
		comp  string
		addrs []string
	}{
		{"", TypeStatic, "", nil},
		{"static", TypeStatic, "", nil},
		{"  ", TypeStatic, "", nil},
		{"10.0.0.1:3000", TypeStatic, "", []string{"10.0.0.1:3000"}},
		{"10.0.0.1:3000, 10.0.0.2:3000", TypeStatic, "", []string{"10.0.0.1:3000", "10.0.0.2:3000"}},
		{"static://10.0.0.1:3000", TypeStatic, "", []string{"10.0.0.1:3000"}},
		{"redis://grpc_registry", TypeRedis, "grpc_registry", nil},
		{" REDIS://grpc_registry ", TypeRedis, "grpc_registry", nil},
	}
	for _, c := range ok {
		src, err := ParseSource(c.raw)
		if err != nil {
			t.Fatalf("%q 解析失败: %v", c.raw, err)
		}
		if src.Type != c.typ || src.Component != c.comp || !reflect.DeepEqual(src.Addresses, c.addrs) {
			t.Fatalf("%q 解析结果不符: %+v", c.raw, src)
		}
		if src.Spec().Type != c.typ || src.Spec().Component != c.comp {
			t.Fatalf("%q 转 Spec 不符: %+v", c.raw, src.Spec())
		}
	}

	bad := []string{
		"rediss://grpc_registry", // 未知 scheme
		"consul://x",
		"redis://",    // 缺组件名
		"redis://a,b", // 组件名里有逗号
		"redis://a/b", // 组件名里有斜杠
	}
	for _, raw := range bad {
		if _, err := ParseSource(raw); err == nil {
			t.Fatalf("%q 应解析失败", raw)
		}
	}
}

func TestKeys(t *testing.T) {
	if RegKey("svc") != KeyRegPrefix+"svc" {
		t.Fatalf("RegKey 不一致: %s", RegKey("svc"))
	}
	if SignalKey("svc") != KeySignalPrefix+"svc" {
		t.Fatalf("SignalKey 不一致: %s", SignalKey("svc"))
	}
	if Field(" 10.0.0.1:3000 ") != "10.0.0.1:3000" {
		t.Fatalf("Field 应去掉空白: %q", Field(" 10.0.0.1:3000 "))
	}
}

func TestRecordName(t *testing.T) {
	cases := []struct {
		publishName, ep, want string
	}{
		{"", "10.0.0.1:3000", "10.0.0.1:3000"},
		{"10.0.0.1:3000", "10.0.0.1:3000", "10.0.0.1:3000"},
		{"order-svc", "10.0.0.1:3000", "order-svc@10.0.0.1:3000"},
		{" order-svc ", " 10.0.0.1:3000 ", "order-svc@10.0.0.1:3000"},
	}
	for _, c := range cases {
		if got := RecordName(c.publishName, c.ep); got != c.want {
			t.Fatalf("RecordName(%q,%q) = %q, want %q", c.publishName, c.ep, got, c.want)
		}
	}
}
