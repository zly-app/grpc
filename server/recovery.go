package server

import (
	"context"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/zly-app/zapp/pkg/utils"
	"google.golang.org/grpc"
)

// 存在panic标记
const ctxTagHasPanic = "panic"

// 恢复
func RecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		panicErr := utils.Recover.WrapCall(func() error {
			resp, err = handler(ctx, req)
			return nil
		})
		if panicErr != nil {
			grpc_ctxtags.Extract(ctx).Set(ctxTagHasPanic, struct{}{})
			return nil, panicErr
		}

		return resp, err
	}
}
