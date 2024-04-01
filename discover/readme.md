
# 服务注册与发现

## static

静态注册和发现器. 让客户端主动设置一个或多个服务节点的属性. 如:

+ ```localhost:3000```
+ ```localhost:3000?weight=100```
+ ```localhost:3000?weight=100&name=service1```
+ ```grpc://localhost:3000?weight=100&name=service1```
+ ```grpc://localhost:3001?weight=100&name=service1,grpc://localhost:3002?weight=100&name=service2```

说明

+ `grpc://` 表示协议类型, 如果未设置默认为 `grpc://`.
+ `localhost:3000` 为服务节点地址, 必须设置.
+ `weight` 为该节点设置权重值, 如果未设置默认为 `100`.
+ `name` 为该节点设置服务节点名, 如果未设置默认为服务节点地址

配置示例

```yaml
services:
  grpc:
    Address: '' # 链接地址
    RegistryType: 'redis' # 发现器类型
```

## redis

redis注册器

