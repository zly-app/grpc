# AI_REFERENCE.md · grpc 项目开发参考

> 本文档是 AI 助手在本仓库工作的入口，先读它再动手。架构判据见工作区 `docs/mso/AGENTS.md`（本地副本），
> 本次改造的设计与决策见 `../../docs/plans/2026-11-09-grpc-mso-refactor-design.md` 与 `../../docs/decisions/0003-grpc-mso-refactor.md`。

---

## 1. 项目概述

- **module**：`github.com/zly-app/grpc`；单 module，远程在 GitHub（公开仓库）
- **Go**：1.24.1
- **定位**：给 [zapp](https://github.com/zly-app/zapp) 提供 gRPC 服务端、客户端、HTTP 网关、服务注册与发现
- **架构**：模型自组织架构（MSO），2026-11-09 按五层重建，**不兼容旧版公开 API**
- **依赖**：`google.golang.org/grpc v1.75.1`、`grpc-gateway/v2 v2.27.3`、**`zapp v1.4.6`**、`component/redis` 伪版本、`zbalancer`、`connpool`、OTel

> **zapp 版本状态（2026-11-09 已发布）**：`go.mod` 指向 `v1.4.6`（MSO 重构后的首个 tag，指向 `8cf5a61`），**无 `replace`**。
> 改造期间用临时 `replace` 指向本地 zapp 验证过 build/test 全绿，发布后正式回写；默认 Go 环境下
> `scripts/verify.sh` 退出码 0。`goproxy.cn` 未收录该模块，首次取数需 `GOPROXY=direct`（或已有本地 module cache）。
> 改造后的代码只依赖 zapp 稳定公开契约，不需要 v1.4.6 的新增 API。

---

## 2. 目录结构（层次即机检约束）

```
grpc/
├── foundation/                 0 底座：无业务语义
│   ├── addr/                   地址值对象 + resolver.Address 属性编解码 + URL/端口解析
│   ├── ctxkey/                 横切上下文：target / hashKey / gatewayData / responseSlot
│   ├── wire/                   线缆编解码：OTel trace / CallerMeta / 自定义 metadata key
│   ├── port/                   窄接口：Registry / Discover / StaticTable / Balancer / ServerRunner / Conn
│   └── resolver/               gRPC Resolver 地址广播机制（带终止保障）
├── model/                      1–2 模型层：数据 + 规则所有者，每个包文件头写契约
│   ├── config/                 三份配置与默认值/约束规则、配置 key 常量
│   ├── code/                   gRPC code ↔ zapp codeType 映射、错误码工具
│   ├── balance/                均衡策略表（表驱动）+ Picker
│   ├── registry/               注册中心 + Redis 共享协议 + 静态地址表 + 目录
│   ├── discover/               发现器目录 + static/redis 实现
│   ├── route/                  网关路由表
│   ├── hooks/                  客户端 hook 表
│   ├── connreg/                客户端连接集合
│   └── serverreg/              服务声明表与重名规则
├── usecase/                    3 业务层：Serve / Dial / Forward，只编排
├── adapter/                    4 接口层：协议适配，禁止业务规则
│   ├── grpcserver/             gRPC Server transport + 拦截器 + TLS
│   ├── grpcclient/             gRPC ClientConn transport + 连接池
│   └── httpgw/                 HTTP ServeMux + CORS + 请求过滤 + 响应重写
├── app/                        5 组合根：唯一装配点（New / Options / Service 适配）
├── grpc.go                     6 根包薄门面：只转发，无状态
├── example/                    9 示例
└── protos/                     proto 依赖（validate / google api 等）
```

依赖方向由工作区 `scripts/mso/layers.conf` 的 `[grpc]` 节声明，**新包不声明层次即机检判红**。
旧包 `server/ client/ gateway/ balance/ registry/ discover/ pkg/` 已全部删除，不要按旧路径找代码。

---

## 3. 快速开始

### 3.1 服务端

```go
g := grpc.New(grpc.WithServer())                 // 唯一组合根
app := zapp.NewApp("grpc-server", g.Options()...) // 必须把 Options() 交给 zapp

hello.RegisterHelloServiceServer(g.Server("hello"), new(HelloService)) // 在 app.Run 之前声明
app.Run()
```

### 3.2 客户端

```go
g := grpc.New()
app := zapp.NewApp("grpc-client", g.Options()...)
defer app.Exit()

client := hello.NewHelloServiceClient(g.ClientConn("hello"))
resp, err := client.Say(ctx, &hello.SayReq{Msg: "hello"})
```

### 3.3 HTTP 网关

```go
g := grpc.New(grpc.WithGateway())
app := zapp.NewApp("grpc-gateway", g.Options()...)

c := hello.NewHelloServiceClient(g.GatewayConn("hello"))
_ = hello.RegisterHelloServiceHandlerClient(context.Background(), g.Mux(), c)
app.Run()
```

`Options()` 内含 `AfterInitialize`（完成装配）与 `BeforeExit`（统一关闭）两个实例级 handler；
**不传 `Options()` 会导致组合根未装配**，`ClientConn` 调用会返回明确错误，`Mux()`/`GatewayConn()` 会 panic。

---

## 4. 核心 API（根包 `grpc`）

| API | 说明 |
|---|---|
| `grpc.New(opts ...Option) *App` | 唯一组合根，构造全部模型并注入 port |
| `(*App).Options() []zapp.Option` | 传给 `zapp.NewApp` 的装配选项 |
| `(*App).Server(name, hooks...) grpc.ServiceRegistrar` | 声明具名 server；同名重复声明 panic |
| `(*App).ServerDesc(hooks...)` | 以 pb `ServiceName` 命名 |
| `(*App).ClientConn(name) grpc.ClientConnInterface` | 客户端连接（按需建立） |
| `(*App).GatewayConn(name)` | 注入网关路由 hashKey 的连接 |
| `(*App).Mux() *runtime.ServeMux` | 网关 mux |
| `(*App).Close()` | 统一关闭，幂等 |

**装配选项**

| Option | 说明 |
|---|---|
| `WithServer()` / `WithGateway()` | 启用 gRPC 服务 / HTTP 网关（对应 zapp service `grpc` / `grpc-gateway`） |
| `WithServerHooks(hs...)` | 所有 server 的 unary 拦截器 |
| `WithClientHooks(name, hs...)` / `WithAllClientHooks(hs...)` | 客户端拦截器 |
| `WithRequestIP(fn)` | 覆盖网关 IP 提取策略 |
| `WithoutErrCodeHook()` | 关闭「把 gRPC 错误码规则装到 zapp filter 链」的接壤装配 |

**值类型与工具**：`grpc.ServerHook` / `ClientHook` / `ServiceRegistrar` / `ClientConnInterface` / `Code` / `GatewayData`；
`grpc.Error` / `Errorf` / `GetErrCode`；`grpc.WithTarget` / `WithHashKey`；
`grpc.ServerExtractCustomData` / `ClientInjectCustomData`；`grpc.GetGatewayData(ctx)`。

---

## 5. 旧版迁移对照（不兼容，不提供兼容层）

| 旧 | 新 |
|---|---|
| `grpc.WithService()` | `grpc.New(grpc.WithServer())` + `g.Options()` |
| `grpc.WithGatewayService()` | `grpc.New(grpc.WithGateway())` + `g.Options()` |
| `grpc.Server(name)` | `g.Server(name)` |
| `grpc.ServerDesc()` | `g.ServerDesc()` |
| `grpc.GetClientConn(name)` | `g.ClientConn(name)` |
| `grpc.GetGatewayClientConn(name)` | `g.GatewayConn(name)` |
| `grpc.GetGatewayMux()` | `g.Mux()` |
| `grpc.RegistryClientHook` / `RegistryAllClientHook` | `grpc.WithClientHooks(name, ...)` / `WithAllClientHooks(...)` |
| 包路径 `server/` `client/` `gateway/` `balance/` `registry/` `discover/` `pkg/` | 见第 2 节五层目录 |
| `init()` 隐式注册、空导入启用实现 | 组合根显式 `Install()` 与注入 |

配置 key 与 YAML 结构**未变**。

---

## 6. 配置参考

### 6.1 服务端 `services.grpc.<serverName>`（`model/config.ServerConfig`）

| 字段 | 默认 | 说明 |
|---|---|---|
| `Bind` | `:3000` | 监听地址 |
| `HeartbeatTime` | 20（最小 3） | 心跳秒数 |
| `ReqDataValidate` | true | 启用 protoc-gen-validate 校验 |
| `ReqDataValidateAllField` | false | 校验所有字段 |
| `SendDetailedErrorInProduction` | false | 生产环境返回详细错误 |
| `TLSCertFile` / `TLSKeyFile` | 空 | 服务端 TLS |
| `RegistryAddress` | `static` | `static` 或 `redis://<组件名>` |
| `PublishName` / `PublishAddress` / `PublishWeight` | 空 / 空 / 100 | 公告名、地址、权重 |

### 6.2 客户端 `components.grpc.<clientName>`（`model/config.ClientConfig`）

| 字段 | 默认 |
|---|---|
| `Address` | `localhost:3000`（`static`/直连地址或 `redis://<组件名>`） |
| `Balance` | `weight_consistent_hash` |
| `WaitFirstConn` / `MinIdle` / `MaxIdle` / `MaxActive` | false / 2 / 4 / 10 |
| `BatchIncrement` / `BatchShrink` | 由 MinIdle/MaxIdle 推导 |
| `IdleTimeout` / `MaxConnLifetime` | 3600s（<1 视为不限制） |
| `WaitTimeout` / `ConnectTimeout` / `CheckIdleInterval` | 5s / 5s / 5s |
| `MaxWaitConnCount` | 2000（<1 视为不限制） |
| `ProxyAddress` / `TLSCertFile` / `TLSDomain` | 空 |

### 6.3 网关 `services.grpc-gateway`（`model/config.GatewayConfig`）

| 字段 | 默认 |
|---|---|
| `Bind` / `CloseWait` / `CorsAllowAll` | `:8080` / 3s / true |
| `Route` | `[]RouteConfig{Path, HashKeyByHeader}`，规则由 `model/route` 持有 |

---

## 7. 负载均衡（`model/balance`）

| 名称 | 规则 |
|---|---|
| `round_robin` | 轮询 |
| `weight_random` | 加权随机 |
| `weight_hash` | 加权 hash，按 `WithHashKey` 匹配 |
| `weight_consistent_hash` | 加权一致性 hash（默认），无 key 时降级 |

四种策略用一张只读表驱动，`Install()` 由组合根显式调用一次（替代旧版 4 个 `init()`）。
`WithTarget` / `WithHashKey` 经 `foundation/ctxkey` 隐式传播到 Picker，不逐参数透传。

---

## 8. 注册与发现

- 实现：`model/registry`（注册）、`model/discover`（发现），共享 `model/registry/protocol.go` 的 key 协议。
- 静态地址表由组合根创建**唯一实例**，同时注入注册侧与发现侧；不再有 `DefStatic` 全局单例、不再有读侧反向依赖写侧。
- `StaticTable.Register` / `RedisRegistry.Register` 是**唯一施放入口**：状态生效与通知在同一次调用内完成。
- Redis：注册有效期 30s、续租 10s、发现侧主动重发现 30s（通知失败时的兜底）。
- 详细说明见 `model/registry/readme.md`。

---

## 9. 请求数据校验、追踪与元数据

- 校验：`protoc-gen-validate` 生成的 `Validate()` / `ValidateAll()` 由 `adapter/grpcserver` 的拦截器调用。
- 追踪：OTel `TextMapCarrier` 在 `foundation/wire` 编解码 gRPC metadata；客户端自动注入、服务端自动提取。
- 主调信息：`filter.CallerMeta` 经 `caller_meta` metadata key 传递；`filter.GetCallerMetaByHeader` 用于网关入口。
- 自定义 metadata：`grpc.ClientInjectCustomData` / `ServerExtractCustomData`，key 统一加 `cw_` 前缀后 base64。
- 网关数据：`grpc.GetGatewayData(ctx)`；**取不到时返回空对象而不是 nil**，必须显式处理缺失分支。

---

## 10. 网关响应包装

`adapter/httpgw.ForwardResponseRewriter` 把 proto 响应包装成：

```json
{ "code": 0, "message": "", "data": { /* 原始 proto 响应 */ }, "trace_id": "xxx" }
```

错误时 `code` 为 gRPC 状态码、`message` 为错误信息、`data` 为空。状态与通知在 `foundation/ctxkey.Slot` 同点共享。
请求/响应 JSON 字段名 = proto 字段名（与 json 标签无关）。

---

## 11. 开发注意事项

1. **新包必须在 `scripts/mso/layers.conf` 声明层次**，否则机检判红。
2. **不要在 `init()` 里注册任何东西**，不要新增包级可变全局；装配动作全部收敛到 `app.New()`。
3. 模型层只经 `foundation/port` 的窄接口横向调用，禁止直翻对方内部状态。
4. 业务层（`usecase`）不许写判定规则；接口层（`adapter`）不许写业务规则。
5. 横切上下文走 `foundation/ctxkey` 隐式传播 + 显式读取，禁止逐参数透传；取不到要显式处理。
6. 新增环必须说明终止机制并补「会停」测试；终止组件由底座提供，模型禁止私有实现。
7. `zapp` 的 `service.RegisterCreatorFunc`、`filter` 链仍是包级全局（zapp 自身待定债务），grpc 只在组合根做一次显式接壤，别在模型层碰它们。

---

## 12. proto 编译命令

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

完整说明见根 `readme.md`。

---

## 13. 验证

```powershell
# 仓库内
go build ./... ; go test ./...

# 工作区门禁（含卫生 + MSO 依赖方向机检 + 13 module build/test）
& 'C:\Program Files\Git\bin\bash.exe' ../../scripts/verify.sh
```

单测覆盖：`foundation/addr`、`foundation/ctxkey`、`foundation/resolver`（会停测试）、
`model/{config,code,balance,route,hooks,connreg,serverreg,registry}`、`app`、`example/client`。
重点用例：`model/registry.TestRegisterBroadcastsImmediately`（唯一施放入口）、
`foundation/resolver.TestUpdateStateTerminates`（终止保障）。

---

## 14. 关键源码索引

| 关注点 | 路径 |
|---|---|
| 组合根 / 装配 | `app/app.go`、`app/service.go` |
| 生命周期用例 | `usecase/serve.go`、`usecase/dial.go`、`usecase/forward.go` |
| 服务端 transport | `adapter/grpcserver/server.go`、`interceptor.go` |
| 客户端连接池 | `adapter/grpcclient/factory.go`、`conn.go` |
| 网关 | `adapter/httpgw/gateway.go`、`req_filter.go`、`response.go` |
| 注册 / 发现 | `model/registry/`、`model/discover/` |
| 错误码规则 | `model/code/code.go` |
| 横切上下文 | `foundation/ctxkey/` |
| port 契约 | `foundation/port/port.go` |
| 机检层次声明 | `../../scripts/mso/layers.conf`（`[grpc]` 节） |
