package grpc

import (
	"github.com/zly-app/grpc/server"
)

type ServiceRegistrar = server.ServiceRegistrar
type RegistryGrpcServerHandler = server.RegistryGrpcServerHandler

// 启用grpc服务
var WithService = server.WithService

// 注册grpc服务handler
var RegistryServerHandler = server.RegistryServerHandler

// 获取logger
var GetLogger = server.GetLogger
