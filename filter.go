package grpc

import (
	"context"

	"github.com/zly-app/zapp/filter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func init() {
	old := filter.DefaultGetErrCodeFunc
	filter.DefaultGetErrCodeFunc = func(ctx context.Context, rsp interface{}, err error) (int, string, error) {
		if err != nil {
			if se, ok := err.(interface {
				GRPCStatus() *status.Status
			}); ok {
				code := se.GRPCStatus().Code()
				if code == codes.OK {
					return 0, filter.CodeTypeSuccess, nil
				}
				return int(code), filter.CodeTypeFail, err
			}
		}
		return old(ctx, rsp, err)
	}
}
