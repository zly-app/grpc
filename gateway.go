package grpc

import (
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/zly-app/grpc/gateway"
	"github.com/zly-app/grpc/pkg"
)

type GatewayMux = runtime.ServeMux
type GatewayData = pkg.GatewayData

// 获取网关服务mux
var GetGatewayMux = gateway.GetGatewayMux

// 获取网关数据
var GetGatewayData = pkg.GetGatewayDataByIncoming

// 获取网关clientConn
var GetGatewayClientConn = gateway.GetGatewayClientConn

var WithGatewayService = gateway.WithService
