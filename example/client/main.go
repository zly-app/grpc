package main

import (
	"context"

	"github.com/zly-app/zapp"

	"github.com/zly-app/grpc"
	"github.com/zly-app/grpc/example/pb/hello"
)

func main() {
	g := grpc.New()
	app := zapp.NewApp("grpc-client", g.Options()...)
	defer app.Exit()

	helloClient := hello.NewHelloServiceClient(g.ClientConn("hello"))

	resp, err := helloClient.Say(context.Background(), &hello.SayReq{Msg: "hello"})
	if err != nil {
		app.Fatal(err)
	}
	app.Info("收到结果", resp.GetMsg())
}
