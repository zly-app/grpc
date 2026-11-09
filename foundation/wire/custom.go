package wire

import (
	"context"
	"encoding/base64"

	"google.golang.org/grpc/metadata"
)

// WrapMetadataPrefix 是自定义 metadata key 的统一前缀。
const WrapMetadataPrefix = "cw_"

// ServerExtractCustomData 服务端提取请求方传入的自定义数据。
func ServerExtractCustomData(ctx context.Context, key string) []string {
	md, _ := metadata.FromIncomingContext(ctx)
	return md.Get(customDataKey(key))
}

// ClientInjectCustomData 客户端把自定义数据注入 ctx，下游可读取。
func ClientInjectCustomData(ctx context.Context, key string, value []string) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.New(nil)
	}
	md.Set(customDataKey(key), value...)
	return metadata.NewOutgoingContext(ctx, md)
}

func customDataKey(key string) string {
	return base64.StdEncoding.EncodeToString([]byte(WrapMetadataPrefix + key))
}
