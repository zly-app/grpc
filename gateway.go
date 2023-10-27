package grpc

import (
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/zly-app/grpc/server"
)

type GatewayMux = runtime.ServeMux

// 获取网关服务mux
var GetGatewayMux = server.GetGatewayMux
