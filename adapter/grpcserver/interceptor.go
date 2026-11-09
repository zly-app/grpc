package grpcserver

import (
	"context"

	"github.com/zly-app/zapp/filter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zly-app/grpc/foundation/wire"
	"github.com/zly-app/grpc/model/config"
)

// appFilter 把 zapp 的服务端过滤器链与链路上下文接到 gRPC unary 调用上。
func (s *Server) appFilter(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (interface{}, error) {
	ctx, chain := filter.GetServiceFilter(ctx, string(config.ServiceTypeGRPC)+"."+s.name, info.FullMethod)
	meta := filter.GetCallMeta(ctx)
	meta.AddCallersSkip(3)

	ctx, mdIn := wire.TraceInjectIn(ctx)
	callMeta, _ := wire.CallerMetaFromMD(mdIn)
	ctx = filter.SaveCallerMeta(ctx, filter.CallerMeta{
		CallerInstance: callMeta.CallerInstance,
		CallerEnv:      callMeta.CallerEnv,
		CallerService:  callMeta.CallerService,
		CallerMethod:   callMeta.CallerMethod,
		CalleeService:  string(config.ServiceTypeGRPC) + "/" + s.name,
		CalleeMethod:   info.FullMethod,
	})

	return chain.Handle(ctx, req, func(ctx context.Context, req interface{}) (interface{}, error) {
		ctx, _ = wire.TraceInjectOut(ctx)
		ctx = filter.SaveCallerMeta(ctx, filter.CallerMeta{})
		return handler(ctx, req)
	})
}

// returnErrorInterceptor 把内部错误翻译成 gRPC status；翻译规则归 model/code。
func (s *Server) returnErrorInterceptor() grpc.UnaryServerInterceptor {
	hideUnknown := !s.app.GetConfig().Config().Frame.Debug && !s.conf.SendDetailedErrorInProduction
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (interface{}, error) {
		reply, err := handler(ctx, req)
		if err == nil {
			return reply, nil
		}

		code, codeType, err := s.mapper.Classify(ctx, reply, err)
		if hideUnknown && err != nil && code == int(codes.Unknown) {
			return reply, status.Error(codes.Internal, "service internal error")
		}
		if _, ok := err.(interface{ GRPCStatus() *status.Status }); ok {
			return nil, err
		}
		if translated, ok := s.mapper.ToStatus(codeType, err); ok {
			return nil, translated
		}
		return reply, err
	}
}

// ValidateInterface / ValidateAllInterface 是 protoc-gen-validate 生成代码的契约。
type ValidateInterface interface{ Validate() error }
type ValidateAllInterface interface{ ValidateAll() error }

// ValidateInterceptor 校验请求，遇错立即返回。
func ValidateInterceptor(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (interface{}, error) {
	if v, ok := req.(ValidateInterface); ok {
		if err := v.Validate(); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	return handler(ctx, req)
}

// ValidateAllInterceptor 校验请求的所有字段；生成代码无 ValidateAll 时降级。
func ValidateAllInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (interface{}, error) {
	v, ok := req.(ValidateAllInterface)
	if !ok {
		return ValidateInterceptor(ctx, req, info, handler)
	}
	if err := v.ValidateAll(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return handler(ctx, req)
}
