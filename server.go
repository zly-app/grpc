package grpc

import (
	"context"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/zly-app/zapp"
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/server"
)

type ServeMux = runtime.ServeMux

type ServiceRegistrar = server.ServiceRegistrar

type UnaryServerInfo = grpc.UnaryServerInfo
type UnaryHandler = grpc.UnaryHandler
type RequestHook = func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (resp interface{}, err error)

// 启用grpc服务
var WithService = func(hooks ...RequestHook) zapp.Option {
	wrapHooks := make([]server.RequestHook, len(hooks))
	for i, h := range hooks {
		wrapHooks[i] = h
	}
	return server.WithService(wrapHooks...)
}

// 注册grpc服务handler
var RegistryServerHandler = func(h func(server ServiceRegistrar)) {
	server.RegistryServerHandler(h)
}

// 注册grpc服务网关handler
var RegistryHttpGatewayHandler = func(h func(ctx context.Context, mux *ServeMux, conn *ClientConn) error) {
	server.RegistryHttpGatewayHandler(h)
}

// 获取logger
var GetLogger = server.GetLogger
