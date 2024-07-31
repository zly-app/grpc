
# 服务注册与发现

## static

静态注册和发现器. 让客户端主动设置一个或多个服务节点的属性. 如:

+ ```localhost:3000```
+ ```localhost:3000?weight=100```
+ ```localhost:3000?weight=100&name=service1```
+ ```static://localhost:3000?weight=100&name=service1```
+ ```static://localhost:3001?weight=100&name=service1,localhost:3002?weight=100&name=service2```

配置示例

```yaml
components:
  grpc:
    hello:
      Address: 'static://localhost:3000?weight=100&name=service1'
```

说明

+ `static://` 表示发现服务类型, 如果未设置默认为 `static://`.
+ `localhost:3000` 为服务节点地址, 必须设置.
+ `weight` 为该节点设置权重值, 如果未设置默认为 `100`.
+ `name` 为该节点设置服务节点名, 如果未设置默认为服务节点地址

## redis

redis注册器, 从redis获取服务地址, 每隔30秒自动重新获取服务地址. 格式 `redis://redis组件名`

配置示例

```yaml
components:
  grpc:
    hello:
      Address: 'redis://default'

  redis:
    default:
     # ... 参考 https://github.com/zly-app/component/tree/master/redis
```

说明

`redis://` 表示发现服务类型, `default` 表示 redis 组件名
