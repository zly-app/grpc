package main

import (
	"context"

	"github.com/zly-app/zapp"

	"github.com/zly-app/grpc"
	"github.com/zly-app/grpc/example/pb/hello"
)

var _ hello.HelloServiceServer = (*HelloService)(nil)

type HelloService struct {
	hello.UnimplementedHelloServiceServer
}

func (h *HelloService) Hello(ctx context.Context, req *hello.HelloReq) (*hello.HelloResp, error) {
	log := grpc.GetLogger(ctx) // 获取log
	log.Info("收到请求", req.Msg)
	return &hello.HelloResp{Msg: req.GetMsg() + "world"}, nil
}

func main() {
	app := zapp.NewApp("grpc-server",
		grpc.WithService(), // 启用 grpc 服务
	)

	grpc.RegistryServerHandler(func(server grpc.ServiceRegistrar) {
		hello.RegisterHelloServiceServer(server, new(HelloService)) // 注册 hello 服务
	})

	app.Run()
}
