package server

import (
	"context"

	"github.com/zly-app/zapp/filter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zly-app/grpc/pkg"
)

type filterReq struct {
	Req interface{}
}
type filterRsp struct {
	Rsp interface{}
}

// 接入app filter
func AppFilter(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx, chain := filter.GetServiceFilter(ctx, string(DefaultServiceType), info.FullMethod)
	meta := filter.GetCallMeta(ctx)
	meta.AddCallersSkip(3)

	ctx = pkg.TraceInjectIn(ctx)

	r := &filterReq{Req: req}
	sp, err := chain.Handle(ctx, r, func(ctx context.Context, req interface{}) (rsp interface{}, err error) {
		ctx = pkg.TraceInjectOut(ctx)
		r := req.(*filterReq)
		sp, err := handler(ctx, r.Req)
		if err != nil {
			return nil, err
		}
		return &filterRsp{Rsp: sp}, nil
	})
	if err != nil {
		return nil, err
	}
	rsp := sp.(*filterRsp)
	return rsp.Rsp, nil
}

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
