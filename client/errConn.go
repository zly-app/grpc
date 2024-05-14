package client

import (
	"context"

	"google.golang.org/grpc"
)

type errConn struct {
	err error
}

func (e errConn) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	return e.err
}

func (e errConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, e.err
}

func newErrConn(err error) ClientConnInterface {
	return errConn{err}
}
