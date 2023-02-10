package client

import (
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
)

type ClientHook = grpc.UnaryClientInterceptor

func HookInterceptor(hooks ...ClientHook) grpc.UnaryClientInterceptor {
	return grpc_middleware.ChainUnaryClient(hooks...)
}
