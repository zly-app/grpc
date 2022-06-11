package client

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"

	"github.com/zly-app/grpc/pkg"
)

type GRpcConnPool struct {
	connPool chan *grpc.ClientConn
	conf     *ClientConfig
}

type IGrpcConn interface {
	grpc.ClientConnInterface
	Close() error
}

func NewGrpcConnPool(conf *ClientConfig, connList []*grpc.ClientConn) *GRpcConnPool {
	g := &GRpcConnPool{
		connPool: make(chan *grpc.ClientConn, len(connList)),
		conf:     conf,
	}
	for _, conn := range connList {
		g.connPool <- conn
	}
	return g
}

func (g *GRpcConnPool) Close() error {
	var err error
	for i := 0; i < cap(g.connPool); i++ {
		conn := <-g.connPool
		e := conn.Close()
		if err == nil {
			err = e
		}
	}
	return err
}

func (g *GRpcConnPool) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	waitCtx := ctx
	if g.conf.WaitConnTime > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, time.Duration(g.conf.WaitConnTime)*time.Second)
		defer cancel()
	}

	var conn *grpc.ClientConn
	select {
	case conn = <-g.connPool:
	case <-waitCtx.Done():
		return waitCtx.Err()
	}

	ctx, opts = pkg.InjectTargetToCtx(ctx, opts)
	ctx, opts = pkg.InjectHashKeyToCtx(ctx, opts)
	err := conn.Invoke(ctx, method, args, reply, opts...)
	g.connPool <- conn
	return err
}

func (g *GRpcConnPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, fmt.Errorf("不支持stream")
}
