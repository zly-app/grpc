package serverreg

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

func TestDeclareAndBind(t *testing.T) {
	global := func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error) {
		return nil, nil
	}
	tab := New(global)
	r, err := tab.Declare("hello")
	if err != nil {
		t.Fatal(err)
	}
	desc := &grpc.ServiceDesc{ServiceName: "hello.HelloService"}
	r.RegisterService(desc, "impl-1")
	r.RegisterService(desc, "impl-2") // 同一 server 复用同一个 transport

	entries := tab.Entries()
	if len(entries) != 1 || entries[0].Name != "hello" {
		t.Fatalf("声名表快照不符: %+v", entries)
	}
	if len(entries[0].Bindings) != 2 || len(entries[0].Hooks) != 1 {
		t.Fatalf("绑定或 hook 数不符: %+v", entries[0])
	}
}

func TestDuplicateDeclareFails(t *testing.T) {
	tab := New()
	if _, err := tab.Declare("dup"); err != nil {
		t.Fatal(err)
	}
	if _, err := tab.Declare("dup"); err == nil {
		t.Fatal("同名服务重复声明必须显式失败")
	}
}

func TestServerDescUsesPBName(t *testing.T) {
	tab := New()
	r, err := tab.Declare("")
	if err != nil {
		t.Fatal(err)
	}
	r.RegisterService(&grpc.ServiceDesc{ServiceName: "pkg.Svc"}, nil)

	entries := tab.Entries()
	if len(entries) != 1 || entries[0].Name != "pkg.Svc" {
		t.Fatalf("未命名声明应取 pb ServiceName: %+v", entries)
	}
}

func TestEntriesIsSnapshot(t *testing.T) {
	tab := New()
	entries := tab.Entries()
	if len(entries) != 0 {
		t.Fatal("空表不应有条目")
	}
	entries = append(entries, &Entry{Name: "fake"})
	if len(tab.Entries()) != 0 {
		t.Fatal("Entries 必须返回快照，外部不得改写模型内部状态")
	}
}
