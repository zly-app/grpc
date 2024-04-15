package grpc

import (
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/zly-app/grpc/gateway"
)

type GatewayMux = runtime.ServeMux
type GatewayData = gateway.GatewayData

// 获取网关服务mux
var GetGatewayMux = gateway.GetGatewayMux

// 获取网关数据
var GetGatewayData = gateway.GetGatewayData

// 获取网关clientConn
var GetGatewayClientConn = gateway.GetGatewayClientConn

var WithGatewayService = gateway.WithService
