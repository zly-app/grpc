package usecase

import (
	"context"

	"google.golang.org/grpc"

	"github.com/zly-app/grpc/foundation/ctxkey"
	"github.com/zly-app/grpc/model/hashkey"
)

// Forward 是「HTTP 请求转发为 gRPC 调用」用例：
// 按 hashKey 规则取出一致性 hash 的 key，再委托 Dial 完成调用链。
type Forward struct {
	dial    *Dial
	hashKey *hashkey.Table
}

// NewForward 装配用例。
func NewForward(d *Dial, hk *hashkey.Table) *Forward {
	return &Forward{dial: d, hashKey: hk}
}

// Invoke 由接口层传入 grpc-gateway 的路径模板；未命中规则时不注入 hashKey。
func (f *Forward) Invoke(ctx context.Context, name, method, path string,
	args, reply interface{}, opts ...grpc.CallOption) error {
	if header, ok := f.hashKey.HeaderFor(path); ok {
		gd := ctxkey.GatewayDataFromIncoming(ctx)
		if gd != nil && gd.Headers != nil {
			if key := gd.Headers.Get(header); key != "" {
				ctx = ctxkey.InjectHashKey(ctx, key)
			}
		}
	}
	return f.dial.Conn(name).Invoke(ctx, method, args, reply, opts...)
}
