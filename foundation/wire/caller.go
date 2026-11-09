package wire

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/zly-app/zapp/filter"
	"google.golang.org/grpc/metadata"
)

const mdCallerMetaKey = "caller_meta"

// CallerMetaFromMD 从 incoming metadata 解出主调信息；解不出返回 false。
func CallerMetaFromMD(md metadata.MD) (filter.CallerMeta, bool) {
	ss := md.Get(mdCallerMetaKey)
	if len(ss) == 0 {
		return filter.CallerMeta{}, false
	}
	meta := filter.CallerMeta{}
	err := sonic.UnmarshalString(ss[0], &meta)
	return meta, err == nil
}

// CallerMetaToMD 把主调信息写进 outgoing metadata 副本；序列化失败时不改 ctx。
func CallerMetaToMD(ctx context.Context, md metadata.MD, callerMeta filter.CallerMeta) context.Context {
	text, err := sonic.MarshalString(callerMeta)
	if err != nil {
		return ctx
	}
	md.Set(mdCallerMetaKey, text)
	return metadata.NewOutgoingContext(ctx, md)
}
