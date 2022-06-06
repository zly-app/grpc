package main

import (
	"context"

	"github.com/zly-app/zapp"

	"github.com/zly-app/grpc/client"
	"github.com/zly-app/grpc/example/pb/hello"
)

func main() {
	app := zapp.NewApp("grpc-client")
	defer app.Exit()

	c := client.NewGRpcClientCreator(app) // 获取grpc客户端建造者
	// 注册客户端创造者
	c.RegistryGRpcClientCreator("hello", func(cc client.ClientConnInterface) interface{} {
		return hello.NewHelloServiceClient(cc)
	})
	helloClient := c.GetGRpcClient("hello").(hello.HelloServiceClient) // 获取客户端

	// 调用
	resp, err := helloClient.Hello(context.Background(), &hello.HelloReq{Msg: "hello"})
	if err != nil {
		app.Fatal(resp)
	}
	app.Info("收到结果", resp.GetMsg())
}
