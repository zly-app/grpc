package main

import (
	"context"

	"github.com/zly-app/zapp"

	"github.com/zly-app/grpc"
	"github.com/zly-app/grpc/example/pb/hello"
)

func main() {
	app := zapp.NewApp("grpc-server",
		grpc.WithGatewayService(), // 启用网关服务
	)

	helloClient := hello.NewHelloServiceClient(grpc.GetClientConn("hello")) // 获取客户端. 网关会通过这个client对service发起调用
	_ = hello.RegisterHelloServiceHandlerClient(context.Background(), grpc.GetGatewayMux(), helloClient) // 注册网关

	app.Run()
}
