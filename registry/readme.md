
# 服务注册与发现

## static

将服务注册在内存中. 将配置`RegistryAddress`设为`static`或空字符串.

配置示例

```yaml
services:
  grpc:
    hello:
      # ...
      RegistryAddress: 'static' # 注册地址
      PublishName: '' # 公告名, 在注册中心中定义的名称, 如果为空则自动设为 PublishAddress
      PublishAddress: '' # 公告地址, 在注册中心中定义的地址, 客户端会根据这个地址连接服务端, 如果为空则自动设为 实例ip:BindPort
      PublishWeight: 100 # 公告权重, 默认100
```

## redis

将服务注册在redis中. 将配置`RegistryAddress`设为`redis://redis组件名`
服务注册有效时间默认为30秒, 每隔10秒自动重新注册.

配置示例

```yaml
services:
  grpc:
    hello:
      # ...
      RegistryAddress: 'redis://default' # 注册地址
      PublishName: '' # 公告名, 无需设置, 其值为 "服务名.序号" 其中序号是每次服务启动首次注册时在redis申请的
      PublishAddress: '' # 公告地址, 在注册中心中定义的地址, 客户端会根据这个地址连接服务端, 如果为空则自动设为 实例ip:BindPort
      PublishWeight: 100 # 公告权重, 默认100

components:
  redis:
    default:
      # ... redis配置
```
