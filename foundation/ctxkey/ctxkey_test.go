package ctxkey

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

func TestSelectorRoundTrip(t *testing.T) {
	ctx := context.Background()
	opts := []grpc.CallOption{
		WithTarget("node-1"),
		WithHashKey("user-42"),
		grpc.EmptyCallOption{},
	}

	ctx, rest := InjectTargetFromOpts(ctx, opts)
	if len(rest) != 2 {
		t.Fatalf("注入 target 后剩余 option = %d, want 2", len(rest))
	}
	ctx, rest = InjectHashKeyFromOpts(ctx, rest)
	if len(rest) != 1 {
		t.Fatalf("注入 hashKey 后剩余 option = %d, want 1", len(rest))
	}
	if Target(ctx) != "node-1" || HashKey(ctx) != "user-42" {
		t.Fatalf("横切上下文读取失败: target=%q hashKey=%q", Target(ctx), HashKey(ctx))
	}
}

func TestDirectHashKeyInjection(t *testing.T) {
	ctx := InjectHashKey(context.Background(), "k")
	if HashKey(ctx) != "k" {
		t.Fatalf("hashKey = %q, want k", HashKey(ctx))
	}
}

func TestMissingContextIsExplicit(t *testing.T) {
	ctx := context.Background()
	if Target(ctx) != "" || HashKey(ctx) != "" {
		t.Fatal("无上下文时选择器应显式返回空串")
	}
	if ResponseSlot(ctx) != nil {
		t.Fatal("无上下文时响应槽应为 nil")
	}
	if GatewayDataOf(ctx) != nil {
		t.Fatal("无上下文时网关数据应为 nil")
	}
	if gd := GatewayDataFromIncoming(ctx); gd == nil || gd.Headers != nil {
		t.Fatal("线缆读取取不到数据时应返回空对象，而不是 nil 放行")
	}
}

func TestResponseSlot(t *testing.T) {
	ctx, slot := WithResponseSlot(context.Background())
	slot.Value = "written"
	if got := ResponseSlot(ctx); got == nil || got.Value != "written" {
		t.Fatal("响应槽未在同一请求链路中共享")
	}
}
