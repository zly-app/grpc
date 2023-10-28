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

	helloClient := hello.NewHelloServiceClient(grpc.GetClientConn("hello")) // 获取客户端

	// 调用
	resp, err := helloClient.Hello(context.Background(), &hello.HelloReq{Msg: "hello"})
	if err != nil {
		app.Fatal(err)
	}
	app.Info("收到结果", resp.GetMsg())
}
