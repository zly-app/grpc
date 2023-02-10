package main

import (
	"context"

	"github.com/zly-app/zapp"
	grpc2 "google.golang.org/grpc"

	"github.com/zly-app/grpc"
	"github.com/zly-app/grpc/example/pb/hello"
)

func hook(ctx context.Context, method string, req, reply interface{}, cc *grpc2.ClientConn, invoker grpc2.UnaryInvoker, opts ...grpc2.CallOption) error {
	return invoker(ctx, method, req, reply, cc, opts...)
}

func main() {
	app := zapp.NewApp("grpc-client")
	defer app.Exit()

	c := grpc.NewGRpcClientCreator(app) // 获取grpc客户端建造者
	// 注册客户端创造者
	c.RegistryGRpcClientCreator("hello", func(cc grpc.ClientConnInterface) interface{} {
		return hello.NewHelloServiceClient(cc)
	}, hook)
	helloClient := c.GetGRpcClient("hello").(hello.HelloServiceClient) // 获取客户端

	// 调用
	resp, err := helloClient.Hello(context.Background(), &hello.HelloReq{Msg: "hello"})
	if err != nil {
		app.Fatal(resp)
	}
	app.Info("收到结果", resp.GetMsg())
}
