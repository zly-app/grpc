package main

import (
	"context"

	"github.com/zly-app/zapp"

	"github.com/zly-app/grpc"
	"github.com/zly-app/grpc/example/pb/hello"
)

func main() {
	app := zapp.NewApp("grpc-client")
	defer app.Exit()

	creator := grpc.NewGRpcClientCreator(app)                             // 获取grpc客户端建造者
	client := hello.NewHelloServiceClient(creator.GetClientConn("hello")) // 获取客户端

	// 调用
	resp, err := client.Hello(context.Background(), &hello.HelloReq{Msg: "hello"})
	if err != nil {
		app.Fatal(err)
	}
	app.Info("收到结果", resp.GetMsg())
}
