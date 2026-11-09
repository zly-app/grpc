package hooks

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

func recorder(tag string, out *[]string) ClientHook {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		*out = append(*out, tag)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func TestClientForMergesGlobalFirst(t *testing.T) {
	tab := New()
	var calls []string
	tab.AddAllClient(recorder("all", &calls))
	tab.AddClient("svc", recorder("svc", &calls))

	chain := tab.ClientFor("svc")
	err := chain(context.Background(), "m", nil, nil, nil,
		func(context.Context, string, interface{}, interface{}, *grpc.ClientConn, ...grpc.CallOption) error {
			calls = append(calls, "invoke")
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[0] != "all" || calls[1] != "svc" || calls[2] != "invoke" {
		t.Fatalf("hook 链顺序不符: %v", calls)
	}
}

func TestClientForOtherServiceOnlyGlobal(t *testing.T) {
	tab := New()
	var calls []string
	tab.AddAllClient(recorder("all", &calls))
	tab.AddClient("svc", recorder("svc", &calls))

	chain := tab.ClientFor("other")
	_ = chain(context.Background(), "m", nil, nil, nil,
		func(context.Context, string, interface{}, interface{}, *grpc.ClientConn, ...grpc.CallOption) error {
			calls = append(calls, "invoke")
			return nil
		})
	if len(calls) != 2 || calls[0] != "all" {
		t.Fatalf("未注册专属 hook 的服务只应走全局链: %v", calls)
	}
}
