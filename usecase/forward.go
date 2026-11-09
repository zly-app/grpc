package usecase

import (
	"context"

	"google.golang.org/grpc"

	"github.com/zly-app/grpc/foundation/ctxkey"
	"github.com/zly-app/grpc/model/route"
)

// Forward 是「HTTP 请求转发为 gRPC 调用」用例：
// 按路由模型判定 hashKey，再委托 Dial 完成调用链。
type Forward struct {
	dial   *Dial
	routes *route.Table
}

// NewForward 装配用例。
func NewForward(d *Dial, r *route.Table) *Forward {
	return &Forward{dial: d, routes: r}
}

// Invoke 由接口层传入 HTTP path；path 取不到时按「无路由规则」显式处理。
func (f *Forward) Invoke(ctx context.Context, name, method, path string,
	args, reply interface{}, opts ...grpc.CallOption) error {
	if header, ok := f.routes.HeaderFor(path); ok {
		gd := ctxkey.GatewayDataFromIncoming(ctx)
		if gd != nil && gd.Headers != nil {
			if key := gd.Headers.Get(header); key != "" {
				ctx = ctxkey.InjectHashKey(ctx, key)
			}
		}
	}
	return f.dial.Conn(name).Invoke(ctx, method, args, reply, opts...)
}
