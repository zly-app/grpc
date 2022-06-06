<!-- TOC -->

- [grpc客户端组件](#grpc客户端组件)
- [快速开始](#快速开始)
- [配置文件](#配置文件)

<!-- /TOC -->

---

# grpc客户端组件

> 提供用于 https://github.com/zly-app/zapp 的组件

# 快速开始

1. 创建一个项目

```shell
mkdir client && cd client
go mod init client
```

2. 添加 `hello/hello.proto` 文件

```protobuf
syntax = 'proto3';
package hello; // 决定proto引用路径和rpc路由
option go_package = "server/hello/hello"; // 用于对golang包管理的定位

service helloService{
   rpc Hello(HelloReq) returns (HelloResp);
}

message HelloReq{
   string msg = 1;
}
message HelloResp{
   string msg = 1;
}
```

3. 编译 proto

```shell
protoc \
--go_out=. --go_opt=paths=source_relative \
--go-grpc_out=. --go-grpc_opt=paths=source_relative \
hello/hello.proto
```

4. 添加 `main.go` 文件

```go
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
```

5. 运行

```shell
go mod tidy && go run .
```

# 配置文件

添加配置文件 `configs/default.yaml`. 更多配置参考[这里](./config.go)

```yaml
components:
  grpc:
    hello:
      Address: localhost:3000
```
