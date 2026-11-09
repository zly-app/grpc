package ctxkey

import (
	"context"
	"net/http"

	"github.com/bytedance/sonic"
	"google.golang.org/grpc/metadata"
)

// GatewayMDataKey 是网关数据在线缆上的 metadata key。
const GatewayMDataKey = "gw.data-bin"

// GatewayData 是 HTTP 请求经网关转发时携带的横切上下文。
type GatewayData struct {
	Method   string
	Path     string
	RawQuery string
	RawBody  string
	IP       string
	Headers  http.Header
}

type gatewayDataKey struct{}

// SaveGatewayData 把网关数据挂到 ctx，供下游显式读取。
func SaveGatewayData(ctx context.Context, data *GatewayData) context.Context {
	return context.WithValue(ctx, gatewayDataKey{}, data)
}

// GatewayDataOf 读取本进程内挂上的网关数据；没有上下文时返回 nil，调用方必须处理。
func GatewayDataOf(ctx context.Context) *GatewayData {
	gd, _ := ctx.Value(gatewayDataKey{}).(*GatewayData)
	return gd
}

// GatewayDataFromIncoming 从对端传来的 metadata 解码网关数据；
// 取不到时返回空对象（不是 nil 放行），避免缺失上下文被当成默认值。
func GatewayDataFromIncoming(ctx context.Context) *GatewayData {
	if gd := GatewayDataOf(ctx); gd != nil {
		return gd
	}
	ret := &GatewayData{}
	md, _ := metadata.FromIncomingContext(ctx)
	if ss := md.Get(GatewayMDataKey); len(ss) > 0 {
		_ = sonic.UnmarshalString(ss[0], ret)
	}
	return ret
}
