package client

import (
	"context"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"google.golang.org/grpc"
)

func ctxTagsInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		t := grpc_ctxtags.NewTags()
		newCtx := grpc_ctxtags.SetInContext(ctx, t)
		return invoker(newCtx, method, req, reply, cc, opts...)
	}
}
