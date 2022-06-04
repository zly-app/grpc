
<!-- TOC -->

- [grpc服务](#grpc%E6%9C%8D%E5%8A%A1)
- [先决条件](#%E5%85%88%E5%86%B3%E6%9D%A1%E4%BB%B6)
- [示例项目](#%E7%A4%BA%E4%BE%8B%E9%A1%B9%E7%9B%AE)
- [快速开始](#%E5%BF%AB%E9%80%9F%E5%BC%80%E5%A7%8B)
- [配置文件](#%E9%85%8D%E7%BD%AE%E6%96%87%E4%BB%B6)
- [请求数据校验](#%E8%AF%B7%E6%B1%82%E6%95%B0%E6%8D%AE%E6%A0%A1%E9%AA%8C)

<!-- /TOC -->

---

# grpc服务

> 提供用于 https://github.com/zly-app/zapp 的服务

# 先决条件


1. 安装protoc编译器

从 https://github.com/protocolbuffers/protobuf/releases 下载protoc编译器, 解压 protoc 执行文件到 `$GOPATH/bin/`

2. 安装 ProtoBuffer Golang 支持

```shell
go install github.com/golang/protobuf/protoc-gen-go@latest
```

3. 安装 ProtoBuffer GRpc Golang 支持. [文档](https://grpc.io/docs/languages/go/quickstart/)

```shell
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

4. 数据校验支持

   1. 安装 [protoc-gen-validate](https://github.com/envoyproxy/protoc-gen-validate)

   ```shell
   go install github.com/envoyproxy/protoc-gen-validate@latest
   ```

   2. 获取依赖 proto 文件

   ```shell
   go get github.com/zly-app/grpc@v0.1.0
   ```

# 示例项目

+ [grpc服务端](example/server/main.go)
+ [grpc客户端](example/client/main.go)

# 快速开始

1. 创建一个项目

```shell
mkdir server && cd server
go mod init server
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

	"github.com/zly-app/grpc"
	"github.com/zly-app/zapp"

	"server/hello"
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

	grpc.RegistryServerHandler(func(server grpc.ServiceRegistrar) {
		hello.RegisterHelloServiceServer(server, new(HelloService)) // 注册 hello 服务
	})

	app.Run()
}
```

5. 运行

```shell
go mod tidy && go run .
```

# 配置文件

添加配置文件 `configs/default.yaml`. 更多配置参考[这里](./config.go)

```yaml
services:
   grpc:
      bind: ":3000"
```

# 请求数据校验

我们使用 [protoc-gen-validate](https://github.com/envoyproxy/protoc-gen-validate) 作为数据校验工具

1. 添加 a.proto 文件

```protobuf
syntax = "proto3";
package example.pb;
option go_package = 'example/pb';
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

2. 编译 proto

```shell
protoc \
-I . \
-I $GOPATH/pkg/mod/github.com/zly-app/grpc@0.1.0/protos \
--go_out=. --go_opt=paths=source_relative \
--validate_out="lang=go:." --validate_opt=paths=source_relative \
a.proto
```

3. 让 IDE 自动完成校验参数

需要将 `$GOPATH/pkg/mod/github.com/zly-app/grpc@0.1.0/protos` 添加到 `Protocol Buffers` 的 `Import Paths`
