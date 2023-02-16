package client

import (
	"context"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/zly-app/zapp/pkg/utils"
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/pkg"
)

// 存在panic标记
const ctxTagHasPanic = "panic"

// 恢复
func RecoveryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
		panicErr := utils.Recover.WrapCall(func() error {
			err = invoker(ctx, method, req, reply, cc, opts...)
			return nil
		})
		if panicErr != nil {
			pkg.TracePanic(ctx, panicErr)
			grpc_ctxtags.Extract(ctx).Set(ctxTagHasPanic, struct{}{})
			return panicErr
		}

		return err
	}
}
