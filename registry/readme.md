
# 服务注册与发现

## static

将服务注册在内存中

配置示例

```yaml
services:
  grpc:
    # ...
    RegistryName: '' # 注册器名称
    RegistryType: 'static' # 注册器类型, 支持 static, redis
    PublishName: '' # 公告名, 在注册中心中定义的名称, 如果为空则自动设为 PublishAddress
    PublishAddress: '' # 公告地址, 在注册中心中定义的地址, 客户端会根据这个地址连接服务端, 如果为空则自动设为 实例ip:BindPort
    PublishWeight: 100 # 公告权重, 默认100
```

## redis

将服务注册在redis中. 服务注册有效时间默认为30秒, 每隔10秒自动重新注册.

配置示例

```yaml
services:
  grpc:
    # ...
    RegistryName: 'default' # redis组件的配置名
    RegistryType: 'redis' # 注册器类型
    PublishName: '' # 公告名, 无需设置, 其值为 "服务名.序号" 其中序号是每次服务启动首次注册时在redis申请的
    PublishAddress: '' # 公告地址, 在注册中心中定义的地址, 客户端会根据这个地址连接服务端, 如果为空则自动设为 实例ip:BindPort
    PublishWeight: 100 # 公告权重, 默认100

components:
  redis:
    default:
      # ... redis配置
```
