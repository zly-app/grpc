package main

import (
	"context"
	"sync"
	"testing"

	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/config"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/pkg/zlog"

	"github.com/zly-app/grpc/client"
	"github.com/zly-app/grpc/example/pb/hello"
)

var testApp core.IApp
var testOnce sync.Once

func makeHelloClient(poolSize int) (core.IApp, hello.HelloServiceClient) {
	testOnce.Do(func() {
		grpcConf := client.NewClientConfig()
		grpcConf.Address = "localhost:3000"
		grpcConf.MaxActive = poolSize
		conf := &core.Config{
			Components: map[string]map[string]interface{}{
				"grpc": {
					"hello": grpcConf,
				},
			},
		}
		conf.Frame.Log = zlog.DefaultConfig
		conf.Frame.Log.WriteToStream = false
		app := zapp.NewApp("grpc-test", zapp.WithConfigOption(config.WithConfig(conf)))
		testApp = app
	})

	helloClient := hello.NewHelloServiceClient(client.GetClientConn("hello"))
	return testApp, helloClient
}

func BenchmarkConnBy1Nums(b *testing.B) {
	app, helloClient := makeHelloClient(1)
	defer app.Exit()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := helloClient.Say(context.Background(), &hello.SayReq{Msg: "hello"})
			if err != nil {
				b.Errorf("调用失败: %v", err)
			}
		}
	})
}

func BenchmarkConnBy5Nums(b *testing.B) {
	app, helloClient := makeHelloClient(5)
	defer app.Exit()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := helloClient.Say(context.Background(), &hello.SayReq{Msg: "hello"})
			if err != nil {
				b.Errorf("调用失败: %v", err)
			}
		}
	})
}
