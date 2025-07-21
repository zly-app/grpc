package grpc

import (
	"context"
	"encoding/base64"

	"google.golang.org/grpc/metadata"
)

const WrapMetadataPrefix = "cw_"

// 服务端提取请求方传入的数据
func ServerExtractCustomData(ctx context.Context, key string) []string {
	mdIn, _ := metadata.FromIncomingContext(ctx)
	return mdIn.Get(makeCustomDataKey(key))
}

// client注入数据到ctx, 使用这个ctx请求下游, 下游可以获取到注入的数据
func ClientInjectCustomData(ctx context.Context, key string, value []string) context.Context {
	mdOut, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		// 如果对元数据修改必须使用它的副本
		mdOut = mdOut.Copy()
	} else {
		mdOut = metadata.New(nil)
	}

	mdOut.Set(makeCustomDataKey(key), value...)
	ctx = metadata.NewOutgoingContext(ctx, mdOut)
	return ctx
}

func makeCustomDataKey(key string) string {
	k := WrapMetadataPrefix + key
	return base64.StdEncoding.EncodeToString([]byte(k))
}
