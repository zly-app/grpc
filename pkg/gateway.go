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

func GetGatewayDataByOutgoing(ctx context.Context) (context.Context, *GatewayData) {
	ret := &GatewayData{}
	mdIn, _ := metadata.FromOutgoingContext(ctx)
	s, _ := mdIn[GatewayMDataKey]
	if len(s) > 0 {
		_ = sonic.UnmarshalString(s[0], ret)
	}

	ctx = context.WithValue(ctx, gatewayDataKey{}, ret)
	return ctx, ret
}

func GetGatewayDataByOutgoingRaw(ctx context.Context) (string, bool) {
	mdIn, _ := metadata.FromOutgoingContext(ctx)
	s, _ := mdIn[GatewayMDataKey]
	if len(s) > 0 {
		return s[0], true
	}
	return "", false
}
