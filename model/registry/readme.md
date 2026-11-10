# 服务注册与发现

> 实现位置：注册侧 `model/registry`、发现侧 `model/discover`，
> 两者共享 `model/registry/protocol.go` 的 key 协议；静态地址表由组合根创建唯一实例并注入两侧。
> 配置 key 未变。

## static

将服务注册在内存中。把配置 `RegistryAddress` 设为 `static` 或空字符串。

```yaml
services:
  grpc:
    hello:
      # ...
      RegistryAddress: 'static' # 注册地址
      PublishName: '' # 公告名，留空则自动取服务名
      PublishAddress: '' # 公告地址，留空则自动取 实例ip:BindPort
      PublishWeight: 100 # 公告权重，默认 100
```

注册与地址广播在同一个入口 `StaticTable.Register` 内完成，已连接的客户端会立刻看到新地址。

## redis

把配置 `RegistryAddress` 设为 `redis://redis组件名`。注册有效期（字段级 TTL）30 秒，每 10 秒自动续租；
发现侧每 30 秒主动重发现一次，作为注册信号丢失时的兜底。

```yaml
services:
  grpc:
    hello:
      # ...
      RegistryAddress: 'redis://default' # 注册地址
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

channel grpc:server:signal:<serverName>   payload {"Reg":{...},"IsUnReg":false}
```

- **注册 / 续租**：各一条 `HSETEX key EX 30 FIELDS 1 <endpoint> <json>`，写值与设 TTL 原子完成。
- **注销**：`HDEL` + `PUBLISH`，3 秒超时；删除只是加速下线，失败由 TTL 兜底。
- **发现**：`HGETALL` 纯只读，按 endpoint 去重并按 endpoint 排序，**不做过期判断、不做删除**。
- 过期时间不写在值里：由 Redis 的字段级 TTL 表达，消除注册方与发现方的跨机时钟漂移。

### 前提与契约

| 项 | 要求 |
|---|---|
| Redis | **≥ 8.0**（`HSETEX` 为 8.0 引入；低版本报 unknown command，不提供降级） |
| `PublishAddress` / `Frame.Instance` | **必须每实例唯一**：禁止填服务域名、VIP、部署单元名，否则多进程抢同一 hash field，表现为「注册表只有一条、死实例不下线、值互相覆盖」 |
| `Name` | `"<PublishName>@<endpoint>"`（无 `PublishName` 时为 endpoint），进入一致性 hash 环，**必须跨重启稳定** |

`Frame.Instance` / `PublishAddress` 配错时启动会打 WARN：同一 endpoint 的旧记录仍在被续租，
说明可能有另一个活实例占着同一标识。

### 升级注意

本协议与旧版（自增 seq 作 field、`Deadline` 落在值里、发现侧 `HDel`）**不兼容**，必须全量同步升级。
升级后残留的旧 key（`grpc:server:seq:*`、旧格式的 `grpc:server:reg:*` 字段）需人工清理。
