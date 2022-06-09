package pkg

import (
	"context"

	"google.golang.org/grpc"
)

type targetKey struct{}

type targetOption struct {
	Target string // 目标
	grpc.EmptyCallOption
}

// 指定目标
func WithTarget(name string) grpc.CallOption {
	return targetOption{Target: name}
}

// 将目标注入到ctx
func InjectTargetToCtx(ctx context.Context, opts []grpc.CallOption) (context.Context, []grpc.CallOption) {
	outOpts := make([]grpc.CallOption, 0, len(opts))
	for _, o := range opts {
		if target, ok := o.(targetOption); ok {
			ctx = context.WithValue(ctx, targetKey{}, target)
		} else {
			outOpts = append(outOpts, o)
		}
	}
	return ctx, outOpts
}

// 从ctx获取目标
func GetTargetByCtx(ctx context.Context) string {
	target, _ := ctx.Value(targetKey{}).(targetOption)
	return target.Target
}

type hashKey struct{}
type hashKeyOption struct {
	HashKey string
	grpc.EmptyCallOption
}

// 指定key
func WithHashKey(key string) grpc.CallOption {
	return hashKeyOption{HashKey: key}
}

// 将key注入到ctx
func InjectHashKeyToCtx(ctx context.Context, opts []grpc.CallOption) (context.Context, []grpc.CallOption) {
	outOpts := make([]grpc.CallOption, 0, len(opts))
	for _, o := range opts {
		if key, ok := o.(hashKeyOption); ok {
			ctx = context.WithValue(ctx, hashKey{}, key)
		} else {
			outOpts = append(outOpts, o)
		}
	}
	return ctx, outOpts
}

// 从ctx获取key
func GetHashKeyByCtx(ctx context.Context) string {
	key, _ := ctx.Value(hashKey{}).(hashKeyOption)
	return key.HashKey
}
