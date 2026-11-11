package main

import (
	"testing"

	"github.com/zly-app/grpc"
)

// 组合根可独立构造，不依赖任何外部服务。
func TestCompositionRoot(t *testing.T) {
	g := grpc.New(grpc.WithoutErrCodeHook())
	if g == nil {
		t.Fatal("组合根为空")
	}
	if g.Server("hello") == nil {
		t.Fatal("服务声明器为空")
	}
	if g.ClientConn("hello") == nil {
		t.Fatal("客户端连接为空")
	}
}

// 同名服务重复声明必须显式失败，而不是静默复用别人的 transport。
func TestDuplicateServerNamePanics(t *testing.T) {
	g := grpc.New(grpc.WithoutErrCodeHook())
	_ = g.Server("dup")
	defer func() {
		if recover() == nil {
			t.Fatal("同名服务重复声明应 panic")
		}
	}()
	_ = g.Server("dup")
}
