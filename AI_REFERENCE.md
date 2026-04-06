# AI_REFERENCE.md - gRPC 项目快速开发参考

> 本文档用于帮助 AI 助手快速理解本 gRPC 项目架构和开发规范，无需扫描整个代码仓库

---

## 1. 项目概述

**模块路径**: `github.com/zly-app/grpc`

**Go 版本**: 1.24.1

**核心功能**: 为 [zapp](https://github.com/zly-app/zapp) 框架提供 gRPC 服务支持，包含服务端、客户端、HTTP 网关、服务注册与发现

**核心依赖**:
- `google.golang.org/grpc v1.75.1`
- `github.com/zly-app/zapp v1.4.1`
- `github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.3`
- `github.com/envoyproxy/protoc-gen-validate v1.2.1`

---

## 2. 项目结构

```
grpc/
├── server/          # gRPC 服务端实现
│   ├── grpc.go      # 服务器核心逻辑、拦截器
│   ├── hooks.go     # 服务端 Hook 拦截器
│   ├── config.go    # 服务端配置
│   └── zapp_adapter.go
├── client/          # gRPC 客户端实现
│   ├── conn.go      # 连接池管理
│   ├── hooks.go     # 客户端 Hook 拦截器
│   ├── config.go    # 客户端配置
│   └── zapp_adapter.go
├── gateway/         # HTTP 网关 (grpc-gateway)
│   ├── http_gateway.go
│   ├── config.go    # 网关配置
│   └── zapp_adapter.go
├── balance/         # 负载均衡器
│   ├── round_robin.go
│   ├── weight_random.go
│   ├── weight_hash.go
│   └── weight_consistent_hash.go
├── registry/        # 服务注册器
│   ├── static/      # 静态注册
│   └── redis/       # Redis 注册
├── discover/        # 服务发现器
│   ├── static/      # 静态发现
│   └── redis/       # Redis 发现
├── pkg/             # 公共工具包
│   ├── address.go   # 地址解析
│   └── trace.go     # 链路追踪
├── server.go        # 服务端入口 API
├── client.go        # 客户端入口 API
├── gateway.go       # 网关入口 API
└── filter.go        # 全局过滤器
```

---

## 3. 快速开始 - AI 辅助开发指南

### 3.1 服务端开发

**标准步骤**:

1. **定义 proto** (`pb/hello/hello.proto`):
```protobuf
syntax = 'proto3';
package hello;
option go_package = "your-module/pb/hello";

service HelloService {
  rpc Say(SayReq) returns (SayResp);
}

message SayReq { string msg = 1; }
message SayResp { string msg = 1; }
```

2. **编译 proto**:
```bash
protoc \
-I . -I ${GOPATH}/protos/zly-app/grpc/protos \
--go_out . --go_opt paths=source_relative \
--go-grpc_out . --go-grpc_opt paths=source_relative \
pb/hello/hello.proto
```

3. **实现服务**:
```go
type HelloService struct {
    hello.UnimplementedHelloServiceServer
}

func (h *HelloService) Say(ctx context.Context, req *hello.SayReq) (*hello.SayResp, error) {
    return &hello.SayResp{Msg: req.GetMsg() + "world"}, nil
}
```

4. **注册服务** (`main.go`):
```go
app := zapp.NewApp("grpc-server", grpc.WithService())
hello.RegisterHelloServiceServer(grpc.Server("hello"), new(HelloService))
app.Run()
```

**关键文件**: `server/grpc.go` (NewGRpcServer), `server.go` (WithService, Server)

### 3.2 客户端开发

**标准步骤**:

1. **获取客户端连接**:
```go
helloClient := hello.NewHelloServiceClient(grpc.GetClientConn("hello"))
resp, err := helloClient.Say(context.Background(), &hello.SayReq{Msg: "hello"})
```

2. **配置文件** (`configs/default.yaml`):
```yaml
components:
  grpc:
    hello:
      Address: localhost:3000
      Balance: weight_consistent_hash  # 均衡器
      MinIdle: 2
      MaxIdle: 4
      MaxActive: 10
```

**关键文件**: `client/conn.go` (NewGRpcConn), `client.go` (GetClientConn, WithHashKey)

### 3.3 HTTP 网关开发

**步骤**:

1. **修改 proto 添加 HTTP 映射**:
```protobuf
import "google/api/annotations.proto";

service HelloService {
  rpc Say(SayReq) returns (SayResp) {
    option (google.api.http) = {
      post: "/hello/say"
      body: "*"
    };
  };
}
```

2. **编译添加网关**:
```bash
--grpc-gateway_out . --grpc-gateway_opt paths=source_relative
```

3. **注册网关**:
```go
app := zapp.NewApp("grpc-gateway", grpc.WithGatewayService())
helloClient := hello.NewHelloServiceClient(grpc.GetGatewayClientConn("hello"))
hello.RegisterHelloServiceHandlerClient(context.Background(), grpc.GetGatewayMux(), helloClient)
app.Run()
```

**关键文件**: `gateway/http_gateway.go`, `gateway.go`

---

## 4. 核心 API 参考

### 4.1 服务端 API (`grpc/server.go`)

| 函数 | 说明 |
|------|------|
| `grpc.WithService(hooks ...ServerHook)` | 启用 gRPC 服务 |
| `grpc.Server(serverName string, hooks ...ServerHook)` | 获取服务注册器 |
| `grpc.ServerDesc(hooks ...ServerHook)` | 获取服务注册器 (无服务名) |

**ServerHook 类型**:
```go
type ServerHook = func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (resp interface{}, err error)
```

### 4.2 客户端 API (`grpc/client.go`)

| 函数 | 说明 |
|------|------|
| `grpc.GetClientConn(clientName string)` | 获取客户端连接 |
| `grpc.WithTarget(serverName string)` | 指定目标服务器 |
| `grpc.WithHashKey(hashKey string)` | 设置负载均衡 hashKey |
| `grpc.RegistryClientHook(hook)` | 注册客户端 Hook |

**ClientHook 类型**:
```go
type ClientHook = func(ctx context.Context, method string, req, reply interface{}, cc *ClientConn, invoker UnaryInvoker, opts ...CallOption) error
```

### 4.3 网关 API (`grpc/gateway.go`)

| 函数 | 说明 |
|------|------|
| `grpc.WithGatewayService()` | 启用 HTTP 网关服务 |
| `grpc.GetGatewayClientConn(clientName string)` | 获取网关客户端连接 |
| `grpc.GetGatewayMux()` | 获取网关 Mux |

---

## 5. 配置参考

### 5.1 服务端配置 (`server/config.go`)

```yaml
services:
  grpc:
    serverName:
      Bind: :3000                           # 监听地址
      HeartbeatTime: 20                     # 心跳时间 (秒)
      ReqDataValidate: true                 # 启用请求数据校验
      ReqDataValidateAllField: false        # 校验所有字段
      SendDetailedErrorInProduction: false  # 生产环境返回详细错误
      TLSCertFile: ''                       # TLS 证书
      TLSKeyFile: ''                        # TLS 私钥
      RegistryAddress: 'static'             # 注册器类型
      PublishName: ''                       # 注册名称
      PublishAddress: ''                    # 注册地址
      PublishWeight: 100                    # 权重
```

### 5.2 客户端配置 (`client/config.go`)

```yaml
components:
  grpc:
    clientName:
      Address: localhost:3000      # 服务地址
      Balance: weight_consistent_hash  # 均衡器类型
      WaitFirstConn: false         # 等待首个连接
      MinIdle: 2                   # 最小闲置连接
      MaxIdle: 4                   # 最大闲置连接
      MaxActive: 10                # 最大活跃连接
      IdleTimeout: 3600            # 空闲超时 (秒)
      WaitTimeout: 5               # 等待连接超时 (秒)
      ConnectTimeout: 5            # 连接超时 (秒)
      ProxyAddress: ''             # SOCKS5 代理
      TLSCertFile: ''              # TLS 证书
      TLSDomain: ''                # TLS 域名
```

### 5.3 网关配置 (`gateway/config.go`)

```yaml
services:
  grpc-gateway:
    Bind: :8080           # 监听地址
    CloseWait: 3          # 关闭等待时间 (秒)
    CorsAllowAll: true    # 允许跨域
    Route:                # 路由配置
      - Path: /hello/say
        HashKeyByHeader: x-hash-key
```

---

## 6. 负载均衡器 (`balance/`)

| 均衡器 | 说明 | 适用场景 |
|--------|------|----------|
| `round_robin` | 轮询 | 均匀分配请求 |
| `weight_random` | 加权随机 | 按权重随机分配 |
| `weight_hash` | 加权 Hash | 相同 hashKey 路由到相同节点 |
| `weight_consistent_hash` | 加权一致性 Hash (默认) | 节点变更时最小化影响 |

**设置 hashKey**:
```go
resp, err := client.Method(ctx, req, grpc.WithHashKey("user-id-123"))
```

**指定目标**:
```go
resp, err := client.Method(ctx, req, grpc.WithTarget("192.168.1.10:3000"))
```

---

## 7. 服务注册与发现

### 7.1 注册器 (`registry/`)

**内置注册器**:
- `static` - 静态注册 (默认)
- `redis` - Redis 注册

**注册器接口**:
```go
type Registry interface {
    Registry(ctx context.Context, serverName string, addr *pkg.AddrInfo) error
    UnRegistry(ctx context.Context, serverName string)
    Close()
}
```

### 7.2 发现器 (`discover/`)

**内置发现器**:
- `static` - 静态发现
- `redis` - Redis 发现

**发现器接口**:
```go
type Discover interface {
    GetBuilder(ctx context.Context, serverName string) (resolver.Builder, error)
    Close()
}
```

---

## 8. 请求数据校验

使用 [protoc-gen-validate](https://github.com/envoyproxy/protoc-gen-validate):

**Proto 示例**:
```protobuf
import "validate/validate.proto";

message UserReq {
  string name = 1 [(validate.rules).string = {
    min_len: 3,
    max_len: 20,
  }];
  int32 age = 2 [(validate.rules).int32 = {
    gt: 0,
    lt: 150,
  }];
}
```

**编译选项**:
```bash
--validate_out "lang=go:." --validate_opt paths=source_relative
```

---

## 9. 链路追踪 (`pkg/trace.go`)

**追踪注入**:
- 客户端自动注入追踪信息到 gRPC metadata
- 服务端自动提取追踪信息
- 支持跨服务调用链追踪

**Metadata 键**:
- `x-caller-service` - 调用方服务名
- `x-caller-instance` - 调用方实例
- `x-caller-method` - 调用方方法
- `x-trace-id` - 追踪 ID

---

## 10. 开发注意事项

### 10.1 Proto 文件规范
- `package` 决定 proto 引用路径和 RPC 路由
- `option go_package` 用于 Go 包管理定位
- 必须导入 `protos` 目录: `${GOPATH}/protos/zly-app/grpc/protos`

### 10.2 服务端注意事项
- 服务启动后自动注册到注册中心
- 服务关闭前自动取消注册
- 支持自定义 ServerHook 拦截器

### 10.3 客户端注意事项
- 使用连接池管理 gRPC 连接
- 每次请求应重新获取客户端
- 支持自定义 ClientHook 拦截器

### 10.4 网关注意事项
- 请求/响应 JSON 字段名 = proto 字段名 (与 json 标签无关)
- 自动生成 Swagger 文档 (可选)

---

## 11. 常用命令

### 完整编译命令 (Linux)
```bash
protoc \
-I . -I ${GOPATH}/protos/zly-app/grpc/protos \
--go_out . --go_opt paths=source_relative \
--go-grpc_out . --go-grpc_opt paths=source_relative \
--grpc-gateway_out . --grpc-gateway_opt paths=source_relative \
--validate_out "lang=go:." --validate_opt paths=source_relative \
--openapiv2_out . \
./*.proto
```

### 完整编译命令 (PowerShell)
```powershell
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

---

## 12. 示例代码位置

| 示例 | 路径 |
|------|------|
| 服务端 | `example/server/main.go` |
| 客户端 | `example/client/main.go` |
| 网关 | `example/gateway/main.go` |
| Proto 示例 | `example/pb/` |

---

## 13. 关键源码索引

| 功能 | 文件位置 |
|------|----------|
| 服务端创建 | `server/grpc.go` |
| 客户端创建 | `client/conn.go` |
| 连接池管理 | `client/conn.go` |
| 服务端拦截器 | `server/grpc.go`, `server/hooks.go` |
| 客户端拦截器 | `client/hooks.go` |
| 负载均衡 | `balance/*.go` |
| 地址解析 | `pkg/address.go` |
| 链路追踪 | `pkg/trace.go` |
| 注册器 | `registry/registry.go` |
| 发现器 | `discover/discover.go` |
