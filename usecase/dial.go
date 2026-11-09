package usecase

import (
	"context"
	"fmt"

	"github.com/zly-app/zapp/filter"
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/foundation/ctxkey"
	"github.com/zly-app/grpc/foundation/wire"
	"github.com/zly-app/grpc/model/config"
	"github.com/zly-app/grpc/model/connreg"
)

// Dial 是「取一条客户端连接并完成调用链编排」用例。
// 连接集合由模型独占，transport 只经 port.Conn 调用。
type Dial struct {
	conns *connreg.Collection
}

// NewDial 装配用例。
func NewDial(conns *connreg.Collection) *Dial { return &Dial{conns: conns} }

// Conn 返回实现 grpc.ClientConnInterface 的编排器；连接按需创建。
func (d *Dial) Conn(name string) grpc.ClientConnInterface {
	return &clientConn{dial: d, name: name}
}

type clientConn struct {
	dial *Dial
	name string
}

func (c *clientConn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	ctx, chain := filter.GetClientFilter(ctx, string(config.ComponentType), c.name, method)
	meta := filter.GetCallMeta(ctx)
	meta.AddCallersSkip(1)

	ctx, _ = wire.TraceInjectIn(ctx)
	return chain.HandleInject(ctx, args, reply, func(ctx context.Context, req, rsp interface{}) error {
		ctx, mdOut := wire.TraceInjectOut(ctx)

		m := filter.GetCallMeta(ctx)
		ctx = wire.CallerMetaToMD(ctx, mdOut, filter.CallerMeta{
			CallerInstance: m.CallerInstance(),
			CallerEnv:      m.CallerEnv(),
			CallerService:  m.CallerService(),
			CallerMethod:   m.CallerMethod(),
		})

		ctx, opts = ctxkey.InjectTargetFromOpts(ctx, opts)
		ctx, opts = ctxkey.InjectHashKeyFromOpts(ctx, opts)

		conn, err := c.dial.conns.Get(c.name)
		if err != nil {
			return err
		}
		err = conn.Invoke(ctx, method, req, rsp, opts...)
		wire.TraceInjectGrpcHeader(ctx, opts...)
		return err
	})
}

func (c *clientConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, fmt.Errorf("当前版本不支持stream")
}
