package grpc

import (
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/client/pkg"
)

// 指定目标
func WithTarget(name string) grpc.CallOption {
	return pkg.WithTarget(name)
}

// 指定key
func WithHashKey(key string) grpc.CallOption {
	return pkg.WithHashKey(key)
}
