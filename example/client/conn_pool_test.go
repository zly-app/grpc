package main

import (
	"context"
	"sync"
	"testing"

	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/config"
	"github.com/zly-app/zapp/core"

	"github.com/zly-app/grpc/client"
	"github.com/zly-app/grpc/example/pb/hello"
)

var testApp core.IApp
var testHelloClient hello.HelloServiceClient
var testOnce sync.Once

func makeHelloClient(poolSize int) (core.IApp, hello.HelloServiceClient) {
	testOnce.Do(func() {
		grpcConf := client.NewClientConfig()
		grpcConf.Address = "localhost:3000"
		grpcConf.ConnPoolCount = poolSize
		conf := &core.Config{
			Components: map[string]map[string]interface{}{
				"grpc": {
					"hello": grpcConf,
				},
			},
		}
		app := zapp.NewApp("grpc-test", zapp.WithConfigOption(config.WithConfig(conf)))

		c := client.NewGRpcClientCreator(app) // 获取grpc客户端建造者
		// 注册客户端创造者
		c.RegistryGRpcClientCreator("hello", func(cc client.ClientConnInterface) interface{} {
			return hello.NewHelloServiceClient(cc)
		})
		helloClient := c.GetGRpcClient("hello").(hello.HelloServiceClient) // 获取客户端
		testApp, testHelloClient = app, helloClient
	})

	return testApp, testHelloClient
}

func BenchmarkConnBy1Nums(b *testing.B) {
	app, helloClient := makeHelloClient(1)
	defer app.Exit()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := helloClient.Hello(context.Background(), &hello.HelloReq{Msg: "hello"})
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
			_, err := helloClient.Hello(context.Background(), &hello.HelloReq{Msg: "hello"})
			if err != nil {
				b.Errorf("调用失败: %v", err)
			}
		}
	})
}
