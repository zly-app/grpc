package grpc

import (
	"context"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/zly-app/grpc/server"
)

type ServeMux = runtime.ServeMux

type ServiceRegistrar = server.ServiceRegistrar

// 启用grpc服务
var WithService = server.WithService

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
