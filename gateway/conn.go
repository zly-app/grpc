package gateway

import (
	"context"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/client"
	"github.com/zly-app/grpc/pkg"
)

type Conn struct {
	cc client.ClientConnInterface
}

func (c *Conn) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	ctx = c.getContext(ctx)
	return c.cc.Invoke(ctx, method, args, reply, opts...)
}

func (c *Conn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	ctx = c.getContext(ctx)
	return c.cc.NewStream(ctx, desc, method, opts...)
}

func (c *Conn) getContext(ctx context.Context) context.Context {
	gd := pkg.GetGatewayData(ctx)
	ctx = c.injectHashKey(ctx, gd)
	return ctx
}

// 注入hashKey用于路由分配
func (c *Conn) injectHashKey(ctx context.Context, gd *pkg.GatewayData) context.Context {
	path, ok := runtime.HTTPPathPattern(ctx)
	if !ok {
		return ctx
	}
	b, ok := defService.conf.GetRouteConfig(path)
	if !ok {
		return ctx
	}
	if b.HashKeyByHeader == "" {
		return ctx
	}
	hashKey := gd.Headers.Get(b.HashKeyByHeader)
	if hashKey == "" {
		return ctx
	}
	return pkg.InjectHashKey(ctx, hashKey)
}

func newConn(serverName string) client.ClientConnInterface {
	cc := client.GetClientConn(serverName)
	return &Conn{cc: cc}
}
