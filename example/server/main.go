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

func (h *HelloService) Say(ctx context.Context, req *hello.SayReq) (*hello.SayResp, error) {
	logger.Log.Info(ctx, "收到请求", req.Msg)
	return &hello.SayResp{Msg: req.GetMsg() + "world"}, nil
}

func main() {
	app := zapp.NewApp("grpc-server",
		grpc.WithService(), // 启用 grpc 服务
	)

	// 注册rpc服务handler
	grpc.RegistryServerHandler(func(ctx context.Context, server grpc.ServiceRegistrar) {
		hello.RegisterHelloServiceServer(server, new(HelloService)) // 注册 hello 服务
	})

	app.Run()
}
