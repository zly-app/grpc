<!-- TOC -->

- [grpc 服务](#grpc%E6%9C%8D%E5%8A%A1)
- [先决条件](#%E5%85%88%E5%86%B3%E6%9D%A1%E4%BB%B6)
- [示例项目](#%E7%A4%BA%E4%BE%8B%E9%A1%B9%E7%9B%AE)
- [快速开始服务端](#%E5%BF%AB%E9%80%9F%E5%BC%80%E5%A7%8B%E6%9C%8D%E5%8A%A1%E7%AB%AF)
- [请求数据校验](#%E8%AF%B7%E6%B1%82%E6%95%B0%E6%8D%AE%E6%A0%A1%E9%AA%8C)
- [客户端](#%E5%AE%A2%E6%88%B7%E7%AB%AF)
- [http 网关](#http%E7%BD%91%E5%85%B3)
- [服务注册与发现](#%E6%9C%8D%E5%8A%A1%E6%B3%A8%E5%86%8C%E4%B8%8E%E5%8F%91%E7%8E%B0)

<!-- /TOC -->

---

# grpc 服务

> 提供用于 https://github.com/zly-app/zapp 的 gRPC 服务端 / 客户端 / HTTP 网关能力。

## 架构（模型自组织架构 MSO，2026-11-09 起）

| 层 | 包 | 职责 |
|---|---|---|
| 0 底座 | `foundation/{addr,ctxkey,wire,port,resolver}` | 纯机制与线缆协议，无业务语义 |
| 1 模型 | `model/{config,code,balance,endpoint,registry,discover,hashkey,hooks,connreg,serverreg}` | 各自独占数据与规则，横向调用只走 port；`endpoint` 是注册/发现共享协议 |
| 3 业务层 | `usecase` | Serve / Dial / Forward 三个用例，只编排不藏规则 |
| 4 接口层 | `adapter/{grpcserver,grpcclient,httpgw}` | 协议适配，禁止业务规则 |
| 5 组合根 | `app` | 唯一装配点：构造全部模型并注入 port |
| 6 门面 | 根包 `grpc` | 只转发，不持有状态 |

**唯一组合根是 `grpc.New()`**：不再有包级可变全局、不再有 `init()` 副作用注册、不再覆写依赖方全局函数。
依赖方向由工作区 `scripts/mso/layers.conf` 锁定，机检保证所有 import 只向下。

> **不兼容旧版**：`grpc.WithService()` / `grpc.WithGatewayService()` / `grpc.Server()` / `grpc.GetClientConn()` /
> `grpc.GetGatewayMux()` / `grpc.GetGatewayClientConn()` 等全局门面已删除，改为组合根实例方法。
> 配置 key 与 YAML 结构保持不变。迁移对照见 [`AI_REFERENCE.md`](./AI_REFERENCE.md)。

## AI 助手开发指南

> **对于 AI 助手**: 在协助开发本 gRPC 项目时，请优先阅读 [`AI_REFERENCE.md`](./AI_REFERENCE.md) 文档，无需扫描整个代码仓库。
>
> 该文档已包含完整开发参考：核心 API、配置项、服务端/客户端/网关快速开始、负载均衡器、注册发现机制、链路追踪、proto 编译命令、关键源码索引等。
>
> **仅在以下情况需要深入阅读源码**:
> - 调试具体问题时
> - 实现高级定制功能时
> - 文档信息不足以解决当前问题时

# 先决条件


1. 安装 protoc 编译器

从 https://github.com/protocolbuffers/protobuf/releases 下载 protoc 编译器，解压 protoc 执行文件到 `${GOPATH}/bin/`

2. 安装 ProtoBuffer Golang 支持

```shell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

3. 安装 ProtoBuffer GRpc Golang 支持。[文档](https://grpc.io/docs/languages/go/quickstart/)

```shell
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

4. 获取依赖 proto 文件

linux

```bash
mkdir -p ${GOPATH}/protos/zly-app && cd ${GOPATH}/protos/zly-app
git clone --depth=1 https://github.com/zly-app/grpc.git
```

Goland 在 `设置` -> `语言和框架` -> `Protocol Buffers/协议缓冲区` 的 `Import Paths`, 取消勾选 `Configure automatically/自动配置`.
将 `${GOPATH}/protos/zly-app/grpc/protos` 添加到 IDE 的 proto 导入路径.

win cmd

```shell
if not exist %GOPATH%\protos\zly-app mkdir %GOPATH%\protos\zly-app
cd /d %GOPATH%\protos\zly-app
git clone --depth=1 https://github.com/zly-app/grpc.git
```

win PowerShell

```shell
if not exist $env:GOPATH\protos\zly-app mkdir $env:GOPATH\protos\zly-app
cd $env:GOPATH\protos\zly-app
git clone --depth=1 https://github.com/zly-app/grpc.git
```

Goland 在 `设置` -> `语言和框架` -> `Protocol Buffers/协议缓冲区` 的 `Import Paths`, 取消勾选 `Configure automatically/自动配置`.
将 `%GOPATH%\protos\zly-app\grpc\protos` 添加到 IDE 的 proto 导入路径.

> 官方 proto 文件参考 https://github.com/googleapis/googleapis/

# 示例项目

+ [grpc 服务端](example/server/main.go)
+ [grpc 客户端](example/client/main.go)
+ [grpc 网关服务](example/gateway/main.go)

# 快速开始 (服务端)

创建工程

```
mkdir grpc-test && cd grpc-test && go mod init grpc-test
```

准备 `pb/hello/hello.proto` 文件

```proto3
syntax = 'proto3';
package hello; // 决定 proto 引用路径和 rpc 路由
option go_package = "grpc-test/pb/hello"; // 用于对 golang 包管理的定位

service helloService{
  rpc Say(SayReq) returns (SayResp);
}

message SayReq{
  string msg = 1;
}
message SayResp{
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

	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/log"

	"github.com/zly-app/grpc"
	"grpc-test/pb/hello"
)

var _ hello.HelloServiceServer = (*HelloService)(nil)

type HelloService struct {
	hello.UnimplementedHelloServiceServer
}

func (h *HelloService) Say(ctx context.Context, req *hello.SayReq) (*hello.SayResp, error) {
	log.Info(ctx, "收到请求", req.Msg)
	return &hello.SayResp{Msg: req.GetMsg() + "world"}, nil
}

func main() {
	// 唯一组合根：先构造实例，再把装配选项交给 zapp
	g := grpc.New(grpc.WithServer())
	app := zapp.NewApp("grpc-server", g.Options()...)

	// 注册 rpc 服务（必须在 app.Run 之前）
	hello.RegisterHelloServiceServer(g.Server("hello"), new(HelloService))

	// 同名服务复用 transport：只取一次注册器，在上面注册多个 pb 服务
	// registrar := g.Server("hello")
	// hello.RegisterHelloServiceServer(registrar, new(HelloService))
	// hello2.RegisterHello2ServiceServer(registrar, new(Hello2Service))
	// 注意：对同一个 serverName 多次调用 g.Server() 会 panic

	app.Run()
}
```

运行服务端

```shell
go mod tidy && go run server/main.go
```

服务端配置文件是可选的

添加配置文件 `configs/default.yaml`.

```yaml
services:
   grpc:
      hello:
         Bind: :3000 # bind 地址
         HeartbeatTime: 20 # 心跳时间，单位秒
         ReqDataValidate: true # 是否启用请求数据校验
         ReqDataValidateAllField: false # 是否对请求数据校验所有字段。如果设为 true, 会对所有字段校验并返回所有的错误。如果设为 false, 校验错误会立即返回.
         SendDetailedErrorInProduction: false # 在生产环境发送详细的错误到客户端。如果设为 false, 在生产环境且错误状态码为 Unknown, 则会返回 service   internal error 给客户端.
         TLSCertFile: '' # tls 公钥文件路径
         TLSKeyFile: '' # tls 私钥文件路径

         RegistryAddress: 'static' # 注册来源：static（只在本进程内可见）或 redis://<components.redis 组件名>
         PublishName: '' # 公告名，在注册中心中定义的名称，如果为空则自动设为当前 grpc 服务名
         PublishAddress: '' # 公告地址，客户端按它连接；留空则按 Bind 的具体主机 → Frame.Instance(IP) → 本机 IP 推导，端口取实际监听端口
         PublishWeight: 100 # 公告权重，默认 100
         ```

# 请求数据校验

我们使用 [protoc-gen-validate](https://github.com/envoyproxy/protoc-gen-validate) 作为数据校验工具

安装 [protoc-gen-validate](https://github.com/envoyproxy/protoc-gen-validate)

```shell
go install github.com/envoyproxy/protoc-gen-validate@latest
```

添加 `pb/a.proto` 示例文件

```protobuf
syntax = "proto3";
package pb; // 决定 proto 引用路径和 rpc 路由
option go_package = "grpc-test/pb"; // 用于对 golang 包管理的定位
import "validate/validate.proto";

message A {
  // 字符串
  string a = 1 [(validate.rules).string = {
    ignore_empty: true, // 可以是空字符串
    //    len: 11, // 长度必须为 11
    max_len: 20, // rune 长度最大为 20
    min_len: 5, // rune 长度最小为 5
    prefix: 'hello', // 前缀
    suffix: 'world', // 后缀
    contains: 'hello world' // 包含字符串
  }];
  // 数字
  int32 b = 2 [(validate.rules).int32 = {
    ignore_empty: true, // 可以是 0
    //    lte: 10, // 必须小等于 10
    lt: 10, // 必须小于 10
    gt: 3, // 必须大于 3
    //    gte: 3, // 必须大于等于 3
    //    const: 5, // 必须等于 5
  }];
  // 布尔型
  bool c = 3[(validate.rules).bool = {
    const: true, // 必须为 true
  }];
  // 数组
  repeated string d = 4[(validate.rules).repeated = {
    max_items: 3, // 最多包含 3 个数据
    min_items: 2, // 最多包含 2 个数据
    unique: true, // 内部数据不允许重复
    items: {
      string: {
        // ... string 选项
      }
    }
  }];
}
```

编译 proto

linux

```shell
protoc \
-I . \
-I ${GOPATH}/protos/zly-app/grpc/protos \
--go_out . --go_opt paths=source_relative \
--validate_out "lang=go:." --validate_opt paths=source_relative \
pb/a.proto
```

win cmd

```shell
protoc ^
-I . ^
-I %GOPATH%/protos/zly-app/grpc/protos ^
--go_out . --go_opt paths=source_relative ^
--validate_out "lang=go:." --validate_opt paths=source_relative ^
pb/a.proto
```

win PowerShell

```shell
protoc `
-I . `
-I $env:GOPATH/protos/zly-app/grpc/protos `
--go_out . --go_opt paths=source_relative `
--validate_out "lang=go:." --validate_opt paths=source_relative `
pb/a.proto
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
	g := grpc.New()
	app := zapp.NewApp("grpc-client", g.Options()...)
	defer app.Exit()

	helloClient := hello.NewHelloServiceClient(g.ClientConn("hello")) // 获取客户端

	// 调用
	resp, err := helloClient.Say(context.Background(), &hello.SayReq{Msg: "hello"})
	if err != nil {
		app.Fatal(err)
	}
	app.Info("收到结果", resp.GetMsg())
}
```

运行客户端

```shell
go mod tidy && go run server/main.go
```

客户端的连接池、均衡器与配置项说明见 [`AI_REFERENCE.md`](./AI_REFERENCE.md)。

# http 网关

使用 [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway) 作为 http 网关

安装 grpc-gateway

```shell
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
```

修改 `pb/hello/hello.proto` 文件

```git
+ import "google/api/annotations.proto"; // 添加导入

service helloService{
-  rpc Say(SayReq) returns (SayResp);
+  rpc Say(SayReq) returns (SayResp){ // 修改 rpc 接口
+    option (google.api.http) = {
+      post: "/hello/say"
+      body: "*"
+    };
+  };
}
```

完整文件如下

```proto3
syntax = 'proto3';
package hello; // 决定 proto 引用路径和 rpc 路由
option go_package = "grpc-test/pb/hello"; // 用于对 golang 包管理的定位

import "google/api/annotations.proto";  // 添加导入

service helloService{
  rpc Say(SayReq) returns (SayResp){// 修改 rpc 接口
    option (google.api.http) = {
      post: "/hello/say"
      body: "*"
    };
  };
}

message SayReq{
  string msg = 1;
}
message SayResp{
  string msg = 1;
}
```

重新编译 proto

linux

```shell
protoc \
-I . \
-I ${GOPATH}/protos/zly-app/grpc/protos \
--go_out . --go_opt paths=source_relative \
--go-grpc_out . --go-grpc_opt paths=source_relative \
--grpc-gateway_out . --grpc-gateway_opt paths=source_relative \
pb/hello/hello.proto
```

win cmd

```shell
protoc ^
-I . ^
-I %GOPATH%/protos/zly-app/grpc/protos ^
--go_out . --go_opt paths=source_relative ^
--go-grpc_out . --go-grpc_opt paths=source_relative ^
--grpc-gateway_out . --grpc-gateway_opt paths=source_relative ^
pb/hello/hello.proto
```

win PowerShell

```shell
protoc `
-I . `
-I $env:GOPATH/protos/zly-app/grpc/protos `
--go_out . --go_opt paths=source_relative `
--go-grpc_out . --go-grpc_opt paths=source_relative `
--grpc-gateway_out . --grpc-gateway_opt paths=source_relative `
pb/hello/hello.proto
```

可以看到新出现了一个 `pb/hello/hello.pb.gw.go` 文件

网关服务端 `server/main.go`

```go
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

	helloClient := hello.NewHelloServiceClient(g.GatewayConn("hello")) // 网关通过这个 client 调用 service
	_ = hello.RegisterHelloServiceHandlerClient(context.Background(), g.Mux(), helloClient)

	app.Run()
}
```

运行网关服务端

```shell
go mod tidy && go run gateway/main.go
```

现在可以通过 curl 访问了

```curl
curl -X POST http://localhost:8080/hello/say -d '{"msg": "hello"}'
```

注意。这里请求和返回的 json 字段名完全等于 proto 中定义的 message 字段名，与 json 标签无关

网关配置是可选的

添加配置文件 `configs/default.yaml`.

```yaml
services:
   grpc-gateway:
      Bind: :8080 # bind 地址
      CloseWait: 3 # 关闭前等待处理时间，单位秒
      CorsAllowAll: true # 允许全局跨域

      HashKey: # 一致性 hash key 规则，可省略
         ByHeader: x-user-id # 默认取哪个请求头；省略则不注入 hashKey
         Rules: # 按 path 覆盖默认来源
            - Path: /hello/say # grpc-gateway 注册的路径模板
              ByHeader: x-hash-key
```

`HashKey` 决定网关用哪个请求头的值作为一致性 hash 的 key：规则命中优先于默认；
规则里 `ByHeader` 留空表示该 path 显式不参与 hash；`Path` 必须以 `/` 开头且不可重复，
配置错误启动即失败。只有 `weight_hash` / `weight_consistent_hash` 均衡器会真正使用该 key。

生成 `swagger`

linux

```bash
protoc \
-I . \
-I ${GOPATH}/protos/zly-app/grpc/protos \
--openapiv2_out=enums_as_ints=true:. \
--go_out . --go_opt paths=source_relative \
pb/hello/hello.proto
```

win cmd

```bash
protoc ^
-I . ^
-I %GOPATH%/protos/zly-app/grpc/protos ^
--openapiv2_out=enums_as_ints=true:. ^
--go_out . --go_opt paths=source_relative ^
pb/hello/hello.proto
```

win PowerShell

```bash
protoc `
-I . `
-I $env:GOPATH/protos/zly-app/grpc/protos `
--openapiv2_out=enums_as_ints=true:. `
--go_out . --go_opt paths=source_relative `
pb/hello/hello.proto
```

# 服务注册与发现

转到 [这里](./model/registry/readme.md)（实现位于 `model/registry` 与 `model/discover`）

# 常用完整的生成 pb 命令

## linux

```makefile
# makefile
pb:
	protoc \
    -I . \
    -I ${GOPATH}/protos/zly-app/grpc/protos \
    --go_out . --go_opt paths=source_relative \
    --go-grpc_out . --go-grpc_opt paths=source_relative \
    --grpc-gateway_out . --grpc-gateway_opt paths=source_relative \
    --validate_out "lang=go:." --validate_opt paths=source_relative \
    --openapiv2_out . \
    ./*.proto
```

## win-powershell

```powershell
#makefile.ps1
protoc `
-I . `
-I $env:GOPATH/protos/zly-app/grpc/protos `
--go_out . --go_opt paths=source_relative `
--go-grpc_out . --go-grpc_opt paths=source_relative `
--grpc-gateway_out . --grpc-gateway_opt paths=source_relative `
--validate_out "lang=go:." --validate_opt paths=source_relative `
--openapiv2_out . `
./*.proto
```
