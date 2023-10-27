package grpc

import (
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/zly-app/grpc/gateway"
)

type GatewayMux = runtime.ServeMux

// 获取网关服务mux
var GetGatewayMux = gateway.GetGatewayMux

var WithGatewayService = gateway.WithService
