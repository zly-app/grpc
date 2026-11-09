package main

import (
	"context"

	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/log"

	"github.com/zly-app/grpc"
	"github.com/zly-app/grpc/example/pb/hello"
)

var _ hello.HelloServiceServer = (*HelloService)(nil)

type HelloService struct {
	hello.UnimplementedHelloServiceServer
}

func (h *HelloService) Say(ctx context.Context, req *hello.SayReq) (*hello.SayResp, error) {
	log.Info(ctx, "收到请求", req.Msg)
	return &hello.SayResp{Msg: req.GetMsg() + "world"}, nil
}

func main() {
	// 组合根：先构造实例，再把装配选项交给 zapp
	g := grpc.New(grpc.WithServer())
	app := zapp.NewApp("grpc-server", g.Options()...)

	// 声明 pb 服务（必须在 app.Run 之前）
	hello.RegisterHelloServiceServer(g.Server("hello"), new(HelloService))

	app.Run()
}
