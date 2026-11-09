// Package ctxkey 是底座 —— 横切上下文的隐式传播与显式读取。
//
// 拥有：无（只有 context.Value 的键与 CallOption 载体）。
// 提供：target / hashKey / gatewayData / responseSlot 的注入与读取。
// 允许调用：无上层。禁止：把取不到上下文当成「不限制」继续执行。
package ctxkey

import (
	"context"

	"google.golang.org/grpc"
)

type targetKey struct{}

type targetOption struct {
	Target string
	grpc.EmptyCallOption
}

// WithTarget 指定目标实例，作为 CallOption 随请求走。
func WithTarget(target string) grpc.CallOption {
	return targetOption{Target: target}
}

// InjectTargetFromOpts 把 target 从 CallOption 搬进 ctx，并把它从 opts 中摘掉。
func InjectTargetFromOpts(ctx context.Context, opts []grpc.CallOption) (context.Context, []grpc.CallOption) {
	out := make([]grpc.CallOption, 0, len(opts))
	for _, o := range opts {
		if t, ok := o.(targetOption); ok {
			ctx = context.WithValue(ctx, targetKey{}, t)
		} else {
			out = append(out, o)
		}
	}
	return ctx, out
}

// Target 读取目标；取不到返回空串（表示不指定，由均衡器按规则选点）。
func Target(ctx context.Context) string {
	t, _ := ctx.Value(targetKey{}).(targetOption)
	return t.Target
}

type hashKeyType struct{}

type hashKeyOption struct {
	HashKey string
	grpc.EmptyCallOption
}

// WithHashKey 指定一致性 hash 的 key。
func WithHashKey(key string) grpc.CallOption {
	return hashKeyOption{HashKey: key}
}

// InjectHashKeyFromOpts 把 hashKey 从 CallOption 搬进 ctx，并把它从 opts 中摘掉。
func InjectHashKeyFromOpts(ctx context.Context, opts []grpc.CallOption) (context.Context, []grpc.CallOption) {
	out := make([]grpc.CallOption, 0, len(opts))
	for _, o := range opts {
		if k, ok := o.(hashKeyOption); ok {
			ctx = context.WithValue(ctx, hashKeyType{}, k)
		} else {
			out = append(out, o)
		}
	}
	return ctx, out
}

// InjectHashKey 直接把 hashKey 放进 ctx（网关路由用）。
func InjectHashKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, hashKeyType{}, hashKeyOption{HashKey: key})
}

// HashKey 读取 hashKey；取不到返回空串（均衡器显式降级为不带 key 的策略）。
func HashKey(ctx context.Context) string {
	k, _ := ctx.Value(hashKeyType{}).(hashKeyOption)
	return k.HashKey
}
