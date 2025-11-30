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
				switch code {
				case codes.OK:
					return 0, filter.CodeTypeSuccess, nil
				case codes.DeadlineExceeded:
					return int(code), filter.CodeTypeTimeoutOrCancel, err
				case codes.Aborted:
					return int(code), filter.CodeTypeException, err
				}
				return int(code), filter.CodeTypeFail, err
			}
		}
		code, codeType, err := old(ctx, rsp, err)
		switch codeType {
		case filter.CodeTypeTimeoutOrCancel:
			return int(codes.DeadlineExceeded), codeType, err
		case filter.CodeTypeFail:
			return int(codes.Internal), codeType, err
		case filter.CodeTypeException:
			return int(codes.Aborted), codeType, err
		}
		return code, codeType, err
	}
}
