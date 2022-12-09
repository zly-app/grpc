package server

import (
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
)

type RequestHook = grpc.UnaryServerInterceptor

func HookInterceptor(hooks ...RequestHook) grpc.UnaryServerInterceptor {
	return grpc_middleware.ChainUnaryServer(hooks...)
}
