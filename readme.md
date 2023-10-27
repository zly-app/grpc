<!-- TOC -->

- [grpc服务](#grpc%E6%9C%8D%E5%8A%A1)
- [先决条件](#%E5%85%88%E5%86%B3%E6%9D%A1%E4%BB%B6)
- [示例项目](#%E7%A4%BA%E4%BE%8B%E9%A1%B9%E7%9B%AE)
- [快速开始服务端](#%E5%BF%AB%E9%80%9F%E5%BC%80%E5%A7%8B%E6%9C%8D%E5%8A%A1%E7%AB%AF)
- [配置文件](#%E9%85%8D%E7%BD%AE%E6%96%87%E4%BB%B6)
- [请求数据校验](#%E8%AF%B7%E6%B1%82%E6%95%B0%E6%8D%AE%E6%A0%A1%E9%AA%8C)
- [http网关](#http%E7%BD%91%E5%85%B3)
- [客户端](#%E5%AE%A2%E6%88%B7%E7%AB%AF)

<!-- /TOC -->
---

# grpc服务

> 提供用于 https://github.com/zly-app/zapp 的服务

> 客户端说明转到[这里](./client)

# 先决条件


1. 安装protoc编译器

从 https://github.com/protocolbuffers/protobuf/releases 下载protoc编译器, 解压 protoc 执行文件到 `${GOPATH}/bin/`

2. 安装 ProtoBuffer Golang 支持

```shell
go install github.com/golang/protobuf/protoc-gen-go@latest
```

3. 安装 ProtoBuffer GRpc Golang 支持. [文档](https://grpc.io/docs/languages/go/quickstart/)

```shell
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

4. 获取依赖 proto 文件

```shell
go install github.com/zly-app/grpc@v0.3.0
```

将 `${GOPATH}/pkg/mod/github.com/zly-app/grpc@v0.3.0/protos` 添加到 IDE 的 proto 导入路径.

Goland 在 `设置` -> `语言和框架` -> `Protocol Buffers` 的 `Import Paths`, 需要取消勾选 `Configure automatically` 才能添加路径.


# 示例项目

+ [grpc服务端](example/server/main.go)
+ [grpc客户端](example/client/main.go)

# 快速开始(服务端)

创建工程

```
mkdir grpc-test && cd grpc-test && go mod init grpc-test
```

准备 `pb/hello/hello.proto` 文件

```proto3
syntax = 'proto3';
package hello; // 决定proto引用路径和rpc路由
option go_package = "grpc-test/pb/hello"; // 用于对golang包管理的定位

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

编译 proto

```
protoc \
--go_out . --go_opt paths=source_relative \
--go-grpc_out . --go-grpc_opt paths=source_relative \
pb/hello/hello.proto
```

服务端 `server/main.go`

```go
package main

import (
	"context"

	"github.com/zly-app/grpc"
	"github.com/zly-app/zapp"
	"grpc-test/pb/hello"
)

var _ hello.HelloServiceServer = (*HelloService)(nil)

type HelloService struct {
	hello.UnimplementedHelloServiceServer
}

func (h *HelloService) Hello(ctx context.Context, req *hello.HelloReq) (*hello.HelloResp, error) {
	log := grpc.GetLogger(ctx) // 获取log
	log.Info("收到请求", req.Msg)
	return &hello.HelloResp{Msg: req.GetMsg() + "world"}, nil
}

func main() {
	app := zapp.NewApp("grpc-server",
		grpc.WithService(), // 启用 grpc 服务
	)

   // 注册rpc服务handler
	grpc.RegistryServerHandler(func(server grpc.ServiceRegistrar) {
		hello.RegisterHelloServiceServer(server, new(HelloService)) // 注册 hello 服务
	})

	app.Run()
}
```

运行服务端

```shell
go mod tidy && go run server/main.go
```

# 配置文件

添加配置文件 `configs/default.yaml`.

```yaml
services:
   grpc:
      Bind: :3000 # bind地址
      HttpBind: ':8080' # http绑定地址
      HeartbeatTime: 20 # 心跳时间, 单位秒
      ReqLogLevelIsInfo: true # 是否设置请求日志等级设为info
      RspLogLevelIsInfo: true # 是否设置响应日志等级设为info
      ProcessTimeout: 1 # 处理超时, 单位秒, <1表示不限制
      ReqDataValidate: true # 是否启用请求数据校验
      ReqDataValidateAllField: false # 是否对请求数据校验所有字段. 如果设为true, 会对所有字段校验并返回所有的错误. 如果设为false, 校验错误会立即返回.
      SendDetailedErrorInProduction: false # 在生产环境发送详细的错误到客户端. 如果设为 false, 在生产环境且错误状态码为 Unknown, 则会返回 service internal error 给客户端.
      ThreadCount: 0 # 同时处理请求的goroutine数, 设为0时取逻辑cpu数*2, 设为负数时不作任何限制, 每个请求由独立的线程执行
      MaxReqWaitQueueSize: 10000 # 最大请求等待队列大小
      TLSCertFile: '' # tls公钥文件路径
      TLSKeyFile: '' # tls私钥文件路径
      TLSDomain: '' # tls签发域名
```

# 请求数据校验

我们使用 [protoc-gen-validate](https://github.com/envoyproxy/protoc-gen-validate) 作为数据校验工具

安装 [protoc-gen-validate](https://github.com/envoyproxy/protoc-gen-validate)

```shell
go install github.com/envoyproxy/protoc-gen-validate@latest
```

添加 `pb/a.proto` 文件

```protobuf
syntax = "proto3";
package a; // 决定proto引用路径和rpc路由
option go_package = "grpc-test/pb"; // 用于对golang包管理的定位
import "validate/validate.proto";

message Person {
  uint64 id    = 1 [(validate.rules).uint64.gt    = 999];

  string email = 2 [(validate.rules).string.email = true];

  string name  = 3 [(validate.rules).string = {
    pattern:   "^[^[0-9]A-Za-z]+( [^[0-9]A-Za-z]+)*$",
    max_bytes: 256,
  }];

  Location home = 4 [(validate.rules).message.required = true];

  message Location {
    double lat = 1 [(validate.rules).double = { gte: -90,  lte: 90 }];
    double lng = 2 [(validate.rules).double = { gte: -180, lte: 180 }];
  }
}
```

编译 proto

```shell
protoc \
-I . \
-I ${GOPATH}/pkg/mod/github.com/zly-app/grpc@v0.3.0/protos \
--go_out . --go_opt paths=source_relative \
--validate_out "lang=go:." --validate_opt paths=source_relative \
pb/a.proto
```

# http网关

使用 [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway) 作为 http 网关

安装 grpc-gateway

```shell
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
```

修改 `pb/hello/hello.proto` 文件

```git
+ import "google/api/annotations.proto"; // 添加导入

service helloService{
-  rpc Hello(HelloReq) returns (HelloResp);
+  rpc Hello(HelloReq) returns (HelloResp){ // 修改rpc接口
+    option (google.api.http) = {
+      post: "/hello/hello"
+      body: "*"
+    };
+  };
}
```

完整文件如下

```proto3
syntax = 'proto3';
package hello; // 决定proto引用路径和rpc路由
option go_package = "grpc-test/pb/hello"; // 用于对golang包管理的定位

import "google/api/annotations.proto";  // 添加导入

service helloService{
  rpc Hello(HelloReq) returns (HelloResp){// 修改rpc接口
    option (google.api.http) = {
      post: "/hello/hello"
      body: "*"
    };
  };
}

message HelloReq{
  string msg = 1;
}
message HelloResp{
  string msg = 1;
}
```


重新编译 proto

```shell
protoc \
-I . \
-I ${GOPATH}/pkg/mod/github.com/zly-app/grpc@v0.3.0/protos \
--go_out . --go_opt paths=source_relative \
--go-grpc_out . --go-grpc_opt paths=source_relative \
--grpc-gateway_out . --grpc-gateway_opt paths=source_relative \
pb/hello/hello.proto
```

可以看到新出现了一个 `pb/hello/hello.pb.gw.go` 文件

修改服务端 `server/main.go` 添加代码

```git
// 注册网关服务handler
grpc.RegistryHttpGatewayHandler(hello.RegisterHelloServiceHandler)
```

完整文件如下

```go
package main

import (
	"context"

	"github.com/zly-app/zapp"
	"github.com/zly-app/grpc"
	"grpc-test/pb/hello"
)

var _ hello.HelloServiceServer = (*HelloService)(nil)

type HelloService struct {
	hello.UnimplementedHelloServiceServer
}

func (h *HelloService) Hello(ctx context.Context, req *hello.HelloReq) (*hello.HelloResp, error) {
	log := grpc.GetLogger(ctx) // 获取log
	log.Info("收到请求", req.Msg)
	return &hello.HelloResp{Msg: req.GetMsg() + "world"}, nil
}

func main() {
	app := zapp.NewApp("grpc-server",
		grpc.WithService(), // 启用 grpc 服务
	)

   // 注册rpc服务handler
	grpc.RegistryServerHandler(func(server grpc.ServiceRegistrar) {
		hello.RegisterHelloServiceServer(server, new(HelloService)) // 注册 hello 服务
	})

   // 注册网关服务handler
	grpc.RegistryHttpGatewayHandler(hello.RegisterHelloServiceHandler)

	app.Run()
}
```

运行服务端

```shell
go mod tidy && go run server/main.go
```

现在可以通过curl访问了

```curl
curl -X POST http://localhost:8080/hello/hello -d '{"msg": "hello"}'
```

# 客户端

创建客户端文件 `client/main.go`

```go
package main

import (
	"context"

	"github.com/zly-app/zapp"

	"github.com/zly-app/grpc"
	"grpc-test/pb/hello"
)

func main() {
	app := zapp.NewApp("grpc-client")
	defer app.Exit()

	c := grpc.NewGRpcClientCreator(app) // 获取grpc客户端建造者
	// 注册客户端创造者
	c.RegistryGRpcClientCreator("hello", func(cc grpc.ClientConnInterface) interface{} {
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

运行客户端

```shell
go mod tidy && go run server/main.go
```

更多客户端说明参考[这里](./client/)
