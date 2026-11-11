# 服务注册与发现

> 实现位置：注册侧 `model/registry`、发现侧 `model/discover`，共享协议在 `model/endpoint`
> （key 布局、注册记录、变更信号、地址来源规格、有效期常量）。
> 静态地址表由组合根创建唯一实例并注入两侧。配置 key 未变。

## static

将服务注册在内存中（只对本进程可见）。把配置 `RegistryAddress` 设为 `static`（或留空）。

```yaml
services:
  grpc:
    hello:
      # ...
      RegistryAddress: 'static' # 注册来源：static 或 redis://<组件名>
      PublishName: '' # 公告名，留空则自动取服务名
      PublishAddress: '' # 公告地址，留空则自动取 实例ip:BindPort
      PublishWeight: 100 # 公告权重，默认 100
```

注册与地址广播在同一个入口 `StaticTable.Register` 内完成，已连接的客户端会立刻看到新地址。
注册按 `endpoint` 幂等：同一地址重复注册只更新名字/权重，不会追加第二条
（客户端重试创建连接会重复调用注册，追加语义会让权重翻倍）。

## redis

把 `RegistryAddress` 设为 `redis://<组件名>`（对应 `components.redis.<name>`）。
注册有效期（字段级 TTL）30 秒，每 10 秒自动续租；发现侧每 30 秒主动重发现一次，作为注册信号丢失时的兜底。

```yaml
services:
  grpc:
    hello:
      # ...
      RegistryAddress: 'redis://default' # 组件名对应 components.redis.<name>
      PublishName: '' # 公告名，作为 Name 的可读前缀，不影响唯一性
      PublishAddress: '' # 公告地址，留空则自动取 实例ip:BindPort
      PublishWeight: 100 # 公告权重，默认 100

components:
  redis:
    default:
      # ... redis 配置
```

### 协议

```
key     grpc:server:reg:<serverName>   hash，field = endpoint
field   10.0.0.1:3000
value   {"Name":"hello@10.0.0.1:3000","Endpoint":"10.0.0.1:3000","Weight":100}
ttl     30s（字段级，Redis 负责过期；hash 为空后 key 自动删除）

channel grpc:server:signal:<serverName>   payload {"Record":{...},"IsUnReg":false}
```

| 动作 | 命令 | 说明 |
|---|---|---|
| 注册 | `HSETEX key FNX EX 30 FIELDS 1 <endpoint> <json>` | 返回 0 说明该 endpoint 已有记录，再无条件覆盖写一次并给出冲突提示 |
| 续租 | `HSETEX key FXX EX 30 FIELDS 1 <endpoint> <json>` | 只在字段仍存在时刷新；**返回 0 表示记录已被 TTL 清掉，立即重建并补发注册信号** |
| 注销 | `HDEL key <endpoint>` + `PUBLISH` | 3s 超时；删除只是加速下线，失败由 TTL 兜底 |
| 发现 | `HGETALL key` | 纯只读，按 endpoint 去重排序，**不判过期、不删除** |

### 四条不变量

1. **有效期只在 TTL 里**：不写进值，避免注册方与发现方的跨机时钟漂移。
2. **发现侧纯只读**：过期由 Redis 负责，只读副本与最小权限 ACL 都成立。
3. **信号优先于快照**：全量发现可能比信号旧。发现侧给每次状态变更记版本号，
   拉取前后版本不一致时快照只补不删，防止「快照回滚」把刚注册的实例摘掉。
4. **通知失败不阻断可用性**：`PUBLISH` 失败只记 ERROR（由 30s 重发现兜底），
   注册落库成功即认为注册成功；反过来，续租重建这类「从不存在到存在」的写入必须补发信号，
   否则纯只读的发现侧最长 30s 看不到一个活着的实例。

### 发现侧时序

```
登记 entry → 订阅 channel → 记录版本 → HGETALL → 按版本合并
```

订阅必须在拉快照之前：反过来的话，「快照之后、订阅之前」发生的注册会丢，只能等下一轮重发现。

下线信号应用前会 `HEXISTS` 二次确认：信号可能乱序或重放，而「字段仍存在」说明实例活着，
误删没有信号可以把它加回来。

地址为空时 `Builder` 返回 `discover.ErrNoInstance`，并在 1s 节流内补拉一次，
让「客户端先起、服务后注册」立刻恢复而不是等 30s。

### 前提与契约

| 项 | 要求 |
|---|---|
| Redis | **≥ 8.0**（`HSETEX` 为 8.0 引入；低版本报 unknown command，不提供降级） |
| `PublishAddress` / `Frame.Instance` | **必须每实例唯一**：禁止填服务域名、VIP、部署单元名，否则多进程抢同一 hash field，表现为「注册表只有一条、死实例不下线、值互相覆盖」 |
| `Name` | `"<PublishName>@<endpoint>"`（无 `PublishName` 时为 endpoint），进入一致性 hash 环，**必须跨重启稳定** |

`PublishAddress` 留空时按以下优先级拼「主机:监听端口」：

1. `Bind` 里写死的具体主机（`127.0.0.1`、某内网 IP）—— 监听地址就是可达地址；
2. `Frame.Instance` 能解析成 IP（其默认值就是本机 IP）；
3. 本机 IP —— `Frame.Instance` 被配成实例名（如 `order-svc-1`）时不能当地址用。

`Bind` 为 `:3000` / `0.0.0.0:3000` 这类通配地址时按 2、3 处理。

`Frame.Instance` / `PublishAddress` 配错时启动会打 WARN「该 endpoint 已有注册记录，本次接管覆盖」。
协议里没有可稳定标识进程的字段，「本进程未优雅退出的残留」与「两个活实例共用标识」无法严格区分：

- 崩溃前最后一次续租总在 0~10s 内，旧记录 TTL 必然落在 (20,30]，与「另一个活实例正在续租」的 TTL 分布完全重叠；
- 用 TTL 判定冲突会 100% 误报，因此只告警、不判错、不阻止接管。

是否真的冲突以「注册表条目数 vs 实际实例数」比对为准。避免冲突的可靠办法是让 `PublishAddress` / 端口固定且每实例唯一。

### 升级注意

本协议与旧版（自增 seq 作 field、`Deadline` 落在值里、发现侧 `HDel`）**不兼容**，必须全量同步升级。
升级后残留的旧 key（`grpc:server:seq:*`、旧格式的 `grpc:server:reg:*` 字段）需人工清理。
