package grpcclient

import (
	"context"
	"fmt"

	"github.com/zlyuancn/connpool"
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/foundation/port"
)

// Conn 独占一个连接池，实现 port.Conn。
// filter / trace / 主调信息 / selector 的编排在业务层，这里只做 transport。
type Conn struct {
	pool connpool.IConnectPool
}

func (c *Conn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	conn, err := c.pool.Get(ctx)
	if err != nil {
		return err
	}
	defer c.pool.Put(conn)

	cc, ok := conn.GetConn().(*grpc.ClientConn)
	if !ok {
		return fmt.Errorf("连接池中不是 *grpc.ClientConn")
	}
	return cc.Invoke(ctx, method, args, reply, opts...)
}

func (c *Conn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, fmt.Errorf("当前版本不支持stream")
}

func (c *Conn) Close() error {
	c.pool.Close()
	return nil
}

var _ port.Conn = (*Conn)(nil)
