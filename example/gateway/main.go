package main

import (
	"context"

	"github.com/zly-app/zapp"

	"github.com/zly-app/grpc"
	"github.com/zly-app/grpc/example/pb/hello"
)

func main() {
	g := grpc.New(grpc.WithGateway())
	app := zapp.NewApp("grpc-gateway", g.Options()...)

	helloClient := hello.NewHelloServiceClient(g.GatewayConn("hello"))
	_ = hello.RegisterHelloServiceHandlerClient(context.Background(), g.Mux(), helloClient)

	app.Run()
}
