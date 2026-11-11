# AI_REFERENCE.md · grpc 项目开发参考

> 本文档是 AI 助手在本仓库工作的入口，先读它再动手。架构判据见工作区 `docs/mso/AGENTS.md`（本地副本），
> 本次改造的设计与决策见 `../../docs/plans/2026-11-09-grpc-mso-refactor-design.md` 与 `../../docs/decisions/0003-grpc-mso-refactor.md`。

---

## 1. 项目概述

- **module**：`github.com/zly-app/grpc`；单 module，远程在 GitHub（公开仓库）
- **Go**：1.24.1
- **定位**：给 [zapp](https://github.com/zly-app/zapp) 提供 gRPC 服务端、客户端、HTTP 网关、服务注册与发现
- **架构**：模型自组织架构（MSO），2026-11-09 按五层重建，**不兼容旧版公开 API**
- **依赖**：`google.golang.org/grpc v1.75.1`、`grpc-gateway/v2 v2.27.3`、**`zapp v1.4.6`**、`component/redis` 伪版本、OTel

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
├── model/                      1 模型层：数据 + 规则所有者，每个包文件头写契约
│   ├── config/                 三份配置与默认值/约束规则、配置 key 常量
│   ├── code/                   gRPC code ↔ zapp codeType 映射、错误码工具
│   ├── balance/                均衡策略表（表驱动）+ Picker
│   ├── endpoint/               注册/发现共享协议：key 布局、注册记录、信号、地址来源规格
│   ├── registry/               注册实现 + 静态地址表 + 注册器目录
│   ├── discover/               发现实现（地址状态机）+ 发现器目录
│   ├── hashkey/                网关 hashKey 规则表（path → 请求头）
│   ├── hooks/                  客户端 hook 表
│   ├── connreg/                客户端连接集合
│   └── serverreg/              服务声明表与重名规则
├── usecase/                    3 业务层：Serve / Dial / Forward，只编排
├── adapter/                    4 接口层：协议适配，禁止业务规则
│   ├── grpcserver/             gRPC Server transport + 拦截器 + TLS
│   ├── grpcclient/             gRPC ClientConn transport（单条惰性连接，无池）
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
| `(*App).GatewayConn(name)` | 按网关 hashKey 规则注入 key 的连接 |
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
| 客户端连接池配置（`MinIdle` / `MaxActive` / …） | 已移除，连接池整个去掉（见 6.2） |
| 均衡器名 `round_robin` / `weight_hash` / … | 加 `zly_` 前缀（见第 7 节） |
| 默认均衡器 `weight_consistent_hash` | `zly_least_request`（P2C） |
| `zbalancer` 依赖 | 已移除，策略全部内实现（见第 7 节） |

配置 key 只在网关 hashKey 规则上有变更（旧 `Route` → `HashKey`，见 6.3）。
注册/发现仍是一行来源字符串（`RegistryAddress` / `Address`），只是解析改严：未知 scheme、
`redis://` 缺组件名在启动期报错，不再静默降级（见 6.1/6.2）。

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
| `RegistryAddress` | `static` | 注册来源：`static` 或 `redis://<组件名>`；启动期严格解析，未知 scheme 直接失败 |
| `PublishName` / `PublishAddress` / `PublishWeight` | 空 / 空 / 100 | 公告名、地址、权重 |

`RegistryAddress` 与客户端的 `Address` 用同一套语法、同一个解析器（`endpoint.ParseSource`）：
`static` / `host:port[,host:port]` / `redis://<组件名>`。未知 scheme（如 `rediss://`）、
`redis://` 缺组件名、组件名带逗号或斜杠，都在启动校验期报错，不再「猜不出来就当直连」。

### 6.2 客户端 `components.grpc.<clientName>`（`model/config.ClientConfig`）

| 字段 | 默认 |
|---|---|
| `Address` | `localhost:3000`；`host:port[,host:port]` / `static` / `redis://<组件名>` |
| `Balance` | `zly_least_request`（见第 7 节） |
| `ProxyAddress` / `TLSCertFile` / `TLSDomain` | 空 |

> **客户端没有连接池**。`WaitFirstConn` / `MinIdle` / `MaxIdle` / `MaxActive` / `BatchIncrement` /
> `BatchShrink` / `IdleTimeout` / `WaitTimeout` / `MaxWaitConnCount` / `ConnectTimeout` /
> `MaxConnLifetime` / `CheckIdleInterval` 全部移除，旧配置里写了也会被忽略。
>
> gRPC 的一条 `ClientConn` 本身并发安全、按 HTTP/2 流多路复用，每个实例一条 SubConn 由均衡器负责。
> 再套一层「独占借还」的池会把并发压到池上限（旧默认 10），并把连接数放大成「池大小 × 实例数」。
> 连接是惰性的：真正的 TCP 在第一次 RPC 时建立，失败由 gRPC 自身退避重连。

**从 v0.6 升级要改的配置只有一类：显式写了策略名的 `Balance`。**

| 情况 | 要不要改 | 说明 |
|---|---|---|
| 没写 `Balance` | 不用 | 自动用新默认 `zly_least_request`（P2C） |
| 写了 `weight_consistent_hash` 等旧名 | **必须改** | 加 `zly_` 前缀；启动期即报错并给出新名 |
| 网关依赖 `HashKey` 做用户粘性 | **必须显式配** | 默认 P2C **不读** hashKey，要粘性得写 `Balance: zly_weight_consistent_hash` |
| 服务端 `services.grpc.<name>` | 不用 | `Bind` / `RegistryAddress` / `PublishWeight` / TLS 全部未变 |
| 旧的 12 个连接池字段 | 不用 | 已被忽略，删不删都行，不报错 |

写错策略名的报错长这样（启动期 panic，不是运行期悄悄失败）：

```
grpc客户端 "hello" 配置错误: balancer 已改名: "weight_consistent_hash" → "zly_weight_consistent_hash"（v0.7.0 起策略名统一加 zly_ 前缀）
```

```yaml
services:
  grpc:
    hello:
      RegistryAddress: redis://grpc_registry   # 服务端注册来源

components:
  grpc:
    hello:
      Address: redis://grpc_registry           # 客户端发现来源
      # 直连写法：Address: '10.0.0.1:3000,10.0.0.2:3000'
```

### 6.3 网关 `services.grpc-gateway`（`model/config.GatewayConfig`）

| 字段 | 默认 |
|---|---|
| `Bind` / `CloseWait` / `CorsAllowAll` | `:8080` / 3s / true |
| `HashKey.ByHeader` | 空（不注入 hashKey） |
| `HashKey.Rules` | 空；`[]HashKeyRule{Path, ByHeader}`，规则由 `model/hashkey` 持有 |

```yaml
services:
  grpc-gateway:
    Bind: :8080
    HashKey:
      ByHeader: x-user-id        # 默认来源：未单独声明的 path 都取它
      Rules:
        - Path: /hello/say       # grpc-gateway 注册的路径模板，不是原始 URL
          ByHeader: x-hash-key   # 覆盖默认来源
        - Path: /hello/ping      # ByHeader 留空：该 path 显式不参与 hash
```

取值语义：规则命中优先；规则头名为空表示显式关闭，**不回落默认来源**；未命中规则用默认来源。
头名在启动期按 RFC 7230 token 校验，`Path` 必须以 `/` 开头且不可重复，配置错误直接启动失败。
只有 `zly_weight_hash` / `zly_weight_consistent_hash` 会用这个 key；默认的 `zly_least_request` 与
`zly_round_robin`、`zly_weight_random` 不读它（取了也不影响选点）。

---

## 7. 负载均衡（`model/balance`）

| 名称 | 规则 |
|---|---|
| `zly_least_request` | **默认**。P2C：随机抽两个实例，选「在途请求数 / 权重」更低的一个 |
| `zly_round_robin` | 轮询 |
| `zly_weight_random` | 加权随机 |
| `zly_weight_hash` | 加权 hash，按 `WithHashKey` 匹配；**不是一致性 hash**，扩缩容会全量重映射 |
| `zly_weight_consistent_hash` | ketama 一致性 hash，按 `WithHashKey` 匹配；增删实例只迁移 1/N |

策略实现全在本包内（`strategy.go`），**不依赖第三方均衡库**。原因是三个能力——按权重、
点名实例（`WithTarget`）、ctx 传 hashKey（`WithHashKey`）——gRPC 官方 balancer 都不提供
（`least_request_experimental` 不看权重、`ring_hash_experimental` 的权重只能由 xDS 通过
`internal` 包设置、hashKey 只能走 service config 的 hashPolicy），第三方实现又都绑死自家框架抽象。

实现要点：

- **P2C 带权重**：负载 = `inflight * 1000 / weight`，让 `PublishWeight` 仍然有效；两个候选
  必须不同（允许重复时只有 2 个实例会有一半概率退化成随机）；两者**负载相同时按权重加权随机**
  ——低 QPS 下所有实例 inflight 都是 0，只比 in-flight 会让权重完全失效（真机实测权重
  200:100 会退化成 1.03:1）。
- **一致性 hash 用固定副本数**：`weight == DefWeight` 时 160 个虚拟节点，按权重等比缩放。
  不做「权重即节点数」，避免调权重连带动环精度。
- **hash 类策略无 key 时降级为轮询**（不是随机），分布仍然均匀。
- **balancer 自己实现**而不是用 `balancer/base`：base 每次子连接状态变化都重建 PickerBuilder，
  而轮询计数、hash 环、在途请求数必须活过重建。实例集合不变时不重建环。

**策略名必须带 `zly_` 前缀**：gRPC 的 balancer 注册表是进程级全局表，`round_robin`、`pick_first`
是 grpc-go 内置名，同名注册会静默覆盖掉进程内所有其它 gRPC 客户端的实现。

选点规则的三条硬约束：

- **顺序确定**：Ready 集合按 `name`→线缆地址全序排序后再交给策略，保证同一 hashKey
  在地址集未变时不会漂移。
- **零值兜底**：`addr.Get` 取不到属性时返回零值。`weight=0` 等于「摘掉该实例」，
  `name=""` 会让 hash 环失去意义，所以分别回落到 `addr.DefWeight` 与线缆地址；
  `addr.Parse` 也直接拒绝 `?weight=0`。
- **错误翻译**：未指定 `target` 时「无实例」等价于 `balancer.ErrNoSubConnAvailable`（等新 picker，
  不结束 RPC）；点名 `target` 但实例不在 Ready 集合里则回 `codes.Unavailable`。

`WithTarget` / `WithHashKey` 经 `foundation/ctxkey` 隐式传播到 Picker，不逐参数透传。
`PickResult.Done` 统一挂回调：登记/回收在途计数，为 P2C 提供观测（以后加 EWMA 也从这里挂）。

---

## 8. 注册与发现

- 实现：`model/registry`（注册）、`model/discover`（发现）；共享协议在 `model/endpoint`（key 布局、记录、信号、地址规格、有效期常量），**发现侧不再 import 注册实现**。
- 静态地址表由组合根创建**唯一实例**，同时注入注册侧与发现侧；注册按 endpoint 幂等 upsert（不追加重复地址）。
- `StaticTable.Register` / `RedisRegistry.Register` 是**唯一施放入口**：状态生效与通知在同一次调用内完成。
- Redis（**≥ 8.0**）：注册 `HSETEX ... FNX`（field 被占时按 TTL 判断冲突再覆盖）、续租 `HSETEX ... FXX`（**返回 0 = 记录已被 TTL 清掉 → 重建并补发信号**）、注销 `HDEL` + `PUBLISH`（3s 超时）、发现侧 `HGETALL` **纯只读**（去重排序，不判过期、不删除）、主动重发现 30s。
- **发现侧状态机**：写者串行 + 读者无锁不可变快照；每次变更记版本，`HGETALL` 前后版本不一致时快照**只补不删**（防旧快照回滚掉刚注册的实例）；时序为「登记 → 订阅 → 记版本 → 拉快照」；下线信号应用前 `HEXISTS` 二次确认。
- **失败处置**：`PUBLISH` 失败只记 ERROR（重发现兜底），不阻断启动；续租重建必须补发信号，否则出现「服务活着但发现侧看不到」的黑洞。地址为空时 `Builder` 返回 `discover.ErrNoInstance` 并在 1s 节流内补拉。
- **心跳与重发现自己调度**，不借用共享任务池（池满会把续租推迟到 TTL 之后）。
- Redis 记录：hash `grpc:server:reg:<serverName>`，**field = endpoint**，值 `{Name, Endpoint, Weight}`；`Name` = `"<PublishName>@<endpoint>"`，进入一致性 hash 环，必须跨重启稳定。有效期不落在值里，由字段级 TTL 表达。
- `PublishAddress` 留空时按序推导公告主机：`Bind` 的具体主机 → `Frame.Instance`（可解析为 IP 时）→ 本机 IP。`Frame.Instance` 被配成实例名时不会直接当地址用。
- 契约：`PublishAddress` / `Frame.Instance` **必须每实例唯一**，否则多进程抢同一 field（互相续命、值互相覆盖）。
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
`model/{config,code,balance,endpoint,discover,hashkey,hooks,connreg,serverreg,registry}`、`app`、`example/client`。
并发安全用 `go test -race ./model/...` 验证（发现侧读路径无锁，写路径串行）。

Redis 集成测试（`TestRedis*`）用环境变量门控，默认 skip；需要真实 Redis 8.x（miniredis 不支持字段级 TTL）：

```powershell
$env:GRPC_TEST_REDIS_ADDR='host:port'; $env:GRPC_TEST_REDIS_PASSWORD='xxx'
go test ./model/registry/ ./model/discover/ -run 'TestRedis' -v
```
重点用例：`model/registry.TestRegisterBroadcastsImmediately`（唯一施放入口）、
`foundation/resolver.TestUpdateStateTerminates`（终止保障）。

---

## 14. 关键源码索引

| 关注点 | 路径 |
|---|---|
| 组合根 / 装配 | `app/app.go`、`app/service.go` |
| 生命周期用例 | `usecase/serve.go`、`usecase/dial.go`、`usecase/forward.go` |
| 服务端 transport | `adapter/grpcserver/server.go`、`interceptor.go` |
| 客户端连接 | `adapter/grpcclient/factory.go`、`conn.go` |
| 网关 | `adapter/httpgw/gateway.go`、`req_filter.go`、`response.go` |
| 端点协议（key / 记录 / 信号） | `model/endpoint/` |
| 注册 / 发现 | `model/registry/`、`model/discover/` |
| 错误码规则 | `model/code/code.go` |
| 横切上下文 | `foundation/ctxkey/` |
| port 契约 | `foundation/port/port.go` |
| 机检层次声明 | `../../scripts/mso/layers.conf`（`[grpc]` 节） |
