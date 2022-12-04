package grpc

import (
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/server"
)

type ServiceRegistrar = server.ServiceRegistrar

// 启用grpc服务
var WithService = server.WithService

// 注册grpc服务handler
var RegistryServerHandler = server.RegistryServerHandler

// 注册grpc服务网关handler
var RegistryHttpGatewayHandler = server.RegistryHttpGatewayHandler

// 获取logger
var GetLogger = server.GetLogger

type ClientConn = grpc.ClientConn
