package main

import (
	"context"

	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/logger"

	"github.com/zly-app/grpc"
	"github.com/zly-app/grpc/example/pb/hello"
)

var _ hello.HelloServiceServer = (*HelloService)(nil)

type HelloService struct {
	hello.UnimplementedHelloServiceServer
}

func (h *HelloService) Hello(ctx context.Context, req *hello.HelloReq) (*hello.HelloResp, error) {
	logger.Log.Info(ctx, "收到请求", req.Msg)
	return &hello.HelloResp{Msg: req.GetMsg() + "world"}, nil
}

func hook(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	return handler(ctx, req)
}

func main() {
	app := zapp.NewApp("grpc-server",
		grpc.WithService(hook), // 启用 grpc 服务
	)

	grpc.RegistryServerHandler(func(ctx context.Context, server grpc.ServiceRegistrar) {
		hello.RegisterHelloServiceServer(server, new(HelloService)) // 注册 hello 服务
		_ = hello.RegisterHelloServiceHandlerServer(ctx, grpc.GetGatewayMux(), new(HelloService)) // 为 hello 服务提供网关服务
	})

	app.Run()
}
