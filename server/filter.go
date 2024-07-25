package server

import (
	"context"

	"google.golang.org/grpc"

	"github.com/zly-app/zapp/filter"

	"github.com/zly-app/grpc/pkg"
)

type filterReq struct {
	Req interface{}
}
type filterRsp struct {
	Rsp interface{}
}

// 接入app filter
func (g *GRpcServer) AppFilter(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx, chain := filter.GetServiceFilter(ctx, string(DefaultServiceType)+"."+g.serverName, info.FullMethod)
	meta := filter.GetCallMeta(ctx)
	meta.AddCallersSkip(3)

	ctx, mdIn := pkg.TraceInjectIn(ctx)

	// 获取上游的主调信息并写入, 修改被调信息
	callMeta, _ := pkg.ExtractCallerMetaFromMD(mdIn)
	ctx = filter.SaveCallerMeta(ctx, filter.CallerMeta{
		CallerService: callMeta.CallerService,
		CallerMethod:  callMeta.CallerMethod,
		CalleeService: string(DefaultServiceType) + "/" + g.serverName,
		CalleeMethod:  info.FullMethod,
	})

	r := &filterReq{Req: req}
	sp, err := chain.Handle(ctx, r, func(ctx context.Context, req interface{}) (rsp interface{}, err error) {
		ctx, _ = pkg.TraceInjectOut(ctx)
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
