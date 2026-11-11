package grpcclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"github.com/zly-app/grpc/foundation/port"
)

// Conn 独占一条 gRPC ClientConn，实现 port.Conn。
// filter / trace / 主调信息 / selector 的编排在业务层，这里只做 transport。
type Conn struct {
	cc *grpc.ClientConn
}

func (c *Conn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	return c.cc.Invoke(ctx, method, args, reply, opts...)
}

func (c *Conn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return c.cc.NewStream(ctx, desc, method, opts...)
}

func (c *Conn) Close() error {
	if c.cc == nil {
		return fmt.Errorf("grpc连接未初始化")
	}
	return c.cc.Close()
}

var _ port.Conn = (*Conn)(nil)
