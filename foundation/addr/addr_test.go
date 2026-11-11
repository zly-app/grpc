package addr

import (
	"net"
	"testing"

	"google.golang.org/grpc/resolver"
)

func TestParseDefaults(t *testing.T) {
	info, err := Parse("localhost:3000")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if info.Endpoint != "localhost:3000" || info.Name != "localhost:3000" || info.Weight != 100 {
		t.Fatalf("默认值不符: %+v", info)
	}
}

func TestParseWithQuery(t *testing.T) {
	info, err := Parse("grpc://10.0.0.1:9000?name=node1&weight=50")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if info.Name != "node1" || info.Weight != 50 || info.Endpoint != "10.0.0.1:9000" {
		t.Fatalf("解析结果不符: %+v", info)
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	// weight=0 在均衡器里等于「忽略该实例」，配置写错必须启动期暴露。
	cases := []string{"http://host:1", "grpc://", "grpc://host:1/path", "grpc://host:1?weight=oops", "grpc://host:1?weight=0"}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Fatalf("非法地址 %q 应报错", c)
		}
	}
}

func TestSetGet(t *testing.T) {
	a := Set(resolver.Address{Addr: "x:1"}, &Info{Name: "n", Endpoint: "x:1", Weight: 7})
	got := Get(a)
	if got.Name != "n" || got.Weight != 7 {
		t.Fatalf("属性回读不符: %+v", got)
	}

	// 取不到属性时返回零值而不是 nil，调用方必须处理「无权重」
	empty := Get(resolver.Address{})
	if empty == nil || empty.Weight != 0 || empty.Name != "" {
		t.Fatalf("空属性应返回零值: %+v", empty)
	}
}

func TestBindPort(t *testing.T) {
	ls, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Close()
	want := ls.Addr().(*net.TCPAddr).Port
	if got := BindPort(ls, ":0"); got != want {
		t.Fatalf("端口 = %d, want %d", got, want)
	}
	if got := BindPort(nil, "127.0.0.1:9123"); got != 9123 {
		t.Fatalf("端口回退解析 = %d, want 9123", got)
	}
}
