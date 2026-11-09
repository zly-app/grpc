# 服务注册与发现

> 实现位置（2026-11-09 MSO 改造后）：注册侧 `model/registry`、发现侧 `model/discover`，
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

把配置 `RegistryAddress` 设为 `redis://redis组件名`。注册有效时间默认 30 秒，每 10 秒自动续租；
发现侧每 30 秒主动重发现一次，作为注册信号丢失时的兜底。

```yaml
services:
  grpc:
    hello:
      # ...
      RegistryAddress: 'redis://default' # 注册地址
      PublishName: '' # 公告名，留空其值为「服务名.序号」，序号在首次注册时向 redis 申请
      PublishAddress: '' # 公告地址，留空则自动取 实例ip:BindPort
      PublishWeight: 100 # 公告权重，默认 100

components:
  redis:
    default:
      # ... redis 配置
```

`RedisRegistry.Register` / `Unregister` 是唯一施放入口：写 hash / 删 hash 与发 Pub/Sub 信号在同一次调用内完成。
信号发布失败会显式上抛（注册）或记录并由定时重发现兜底（注销），不再静默忽略。
