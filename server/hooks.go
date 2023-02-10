package server

import (
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
)

type ServerHook = grpc.UnaryServerInterceptor

func HookInterceptor(hooks ...ServerHook) grpc.UnaryServerInterceptor {
	return grpc_middleware.ChainUnaryServer(hooks...)
}
