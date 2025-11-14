package pkg

import (
	"context"
	"net/http"

	"github.com/bytedance/sonic"
	"google.golang.org/grpc/metadata"
)

const (
	GatewayMDataKey = "gw.data-bin"
)

type GatewayData struct {
	Method   string
	Path     string
	RawQuery string
	RawBody  string
	IP       string
	Headers  http.Header
}

type gatewayDataKey struct{}

// 获取网关数据
func GetGatewayDataByIncoming(ctx context.Context) *GatewayData {
	tmp := ctx.Value(gatewayDataKey{})
	if gd, ok := tmp.(*GatewayData); ok {
		return gd
	}

	ret := &GatewayData{}
	mdIn, _ := metadata.FromIncomingContext(ctx)
	s, _ := mdIn[GatewayMDataKey]
	if len(s) > 0 {
		_ = sonic.UnmarshalString(s[0], ret)
	}
	return ret
}

func SaveGatewayData(ctx context.Context, data *GatewayData) context.Context {
	ctx = context.WithValue(ctx, gatewayDataKey{}, data)
	return ctx
}

func GetGatewayData(ctx context.Context) *GatewayData {
	tmp := ctx.Value(gatewayDataKey{})
	if gd, ok := tmp.(*GatewayData); ok {
		return gd
	}
	return nil
}
