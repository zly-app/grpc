<!-- TOC -->

- [grpc客户端组件](#grpc%E5%AE%A2%E6%88%B7%E7%AB%AF%E7%BB%84%E4%BB%B6)
- [快速开始](#%E5%BF%AB%E9%80%9F%E5%BC%80%E5%A7%8B)
- [配置文件](#%E9%85%8D%E7%BD%AE%E6%96%87%E4%BB%B6)
    - [最少配置设置](#%E6%9C%80%E5%B0%91%E9%85%8D%E7%BD%AE%E8%AE%BE%E7%BD%AE)
    - [完整配置说明](#%E5%AE%8C%E6%95%B4%E9%85%8D%E7%BD%AE%E8%AF%B4%E6%98%8E)
- [请求负载均衡](#%E8%AF%B7%E6%B1%82%E8%B4%9F%E8%BD%BD%E5%9D%87%E8%A1%A1)
    - [hashKey 设置方式](#hashkey-%E8%AE%BE%E7%BD%AE%E6%96%B9%E5%BC%8F)
    - [目标选择](#%E7%9B%AE%E6%A0%87%E9%80%89%E6%8B%A9)
- [注册器](#%E6%B3%A8%E5%86%8C%E5%99%A8)
    - [static](#static)

<!-- /TOC -->

---

# grpc客户端组件

> 提供用于 https://github.com/zly-app/zapp 的组件

# 快速开始

1. 创建一个项目

```shell
mkdir client && cd client
go mod init client
```

2. 添加 `hello/hello.proto` 文件

```protobuf
syntax = 'proto3';
package hello; // 决定proto引用路径和rpc路由
option go_package = "server/hello/hello"; // 用于对golang包管理的定位

service helloService{
   rpc Hello(HelloReq) returns (HelloResp);
}

message HelloReq{
   string msg = 1;
}
message HelloResp{
   string msg = 1;
}
```

3. 编译 proto

```shell
protoc \
--go_out=. --go_opt=paths=source_relative \
--go-grpc_out=. --go-grpc_opt=paths=source_relative \
hello/hello.proto
```

4. 添加 `main.go` 文件

```go
package main

import (
	"context"

	"github.com/zly-app/zapp"

	"github.com/zly-app/grpc"
	"github.com/zly-app/grpc/example/pb/hello"
)

func main() {
	app := zapp.NewApp("grpc-client")
	defer app.Exit()

  helloClient := hello.NewHelloServiceClient(grpc.GetClientConn("hello")) // 获取客户端

	// 调用
	resp, err := helloClient.Hello(context.Background(), &hello.HelloReq{Msg: "hello"})
	if err != nil {
		app.Fatal(resp)
	}
	app.Info("收到结果", resp.GetMsg())
}
```

5. 运行

```shell
go mod tidy && go run .
```

# 配置文件

添加配置文件 `configs/default.yaml`.

最少配置设置

```yaml
components:
   grpc:
      hello: # 客户端名
         Address: localhost:3000 # 链接地址
```

完整配置说明

```yaml
components:
   grpc:
      hello: # 服务名
         Address: localhost:3000 # 链接地址
         Registry: static # 注册器, 支持 static
         Balance: weight_consistent_hash # 均衡器, 支持 round_robin, weight_random, weight_hash, weight_consistent_hash
         DisableOpenTrace: false # 是否关闭OpenTrace
         ReqLogLevelIsInfo: true # 是否将请求日志等级设为info
         RspLogLevelIsInfo: true # 是否将响应日志等级设为info
         ReqTimeout: 1 # 请求超时, 单位秒, <1表示不限制
         WaitFirstConn: true # 初始化时等待第一个链接
         MinIdle: 2 # 最小闲置
         MaxIdle: 4 # 最大闲置
         MaxActive: 10 # 最大活跃连接数, 小于1表示不限制
         BatchIncrement: 4 # 批次增量, 当conn不够时, 一次性最多申请多少个链接
         BatchShrink: 4 # 批次缩容, 当conn太多时(超过最大闲置), 一次性最多释放多少个链接
         IdleTimeout: 3600 # 空闲链接超时时间, 单位秒, 如果一个连接长时间未使用将被视为连接无效, 小于1表示永不超时
         WaitTimeout: 5 # 等待获取连接的超时时间, 单位秒
         MaxWaitConnCount: 2000 # 最大等待conn的数量, 小于1表示不限制
         ConnectTimeout: 5 # 连接超时, 单位秒
         MaxConnLifetime: 3600 # 一个连接最大存活时间, 单位秒, 小于1表示不限制
         CheckIdleInterval: 5 # 检查空闲间隔, 单位秒
         ProxyAddress: "" # 代理地址. 支持 socks5, socks5h. 示例: socks5://127.0.0.1:1080 socks5://127.0.0.1:1080 socks5://user:pwd@127.0.0.1:1080
         TLSCertFile: "" # tls公钥文件路径
         TLSDomain: "" # tls签发域名
```

# 请求负载均衡

首先你需要选择一个负载均衡器, 通过配置 Balance 设置, 它提供了如下均衡器

+ round_robin

轮询器. 每次请求会按顺序选择一个服务节点, 节点轮训完毕后会重新从第一个服务节点开始

+ weight_random

加权随机. 允许为节点设置一个权重值, 每次请求会随机选择一个服务节点, 权重越高, 请求时被选中的概率越大.

+ weight_hash

加权 hash. 允许为节点设置一个权重值, 每次请求会根据提供的 `hashKey` 计算 hash 值然后对总权重求余, 余数计算所在的服务节点, 权重越高被选取的机会越大.

在服务节点连接异常时, 会重新编排节点, 导致使用同一个 `hashKey` 的请求落在不同的服务节点上, 推荐使用 `weight_consistent_hash`.

如果在请求时没有设置 `hashKey` 会降级为加权随机.

+ weight_consistent_hash

加权一致性 hash. 允许为节点设置一个权重值, 权重值会作为节点的分片数, 每个分片计算hash值落在一个具有 2^32 个点的环上. 每次请求会根据提供的 `hashKey` 计算 hash 值落在环的一个点上, 由这个点得到是哪个服务节点的分片而选取这个服务节点.

在服务节点连接异常时, 由这个服务节点负责的环上的点会交给别的服务, 所以只会有部分使用同一个 `hashKey` 的请求会落在不同的服务节点上, 当服务节点连接恢复后, 这部分请求会回到原来的服务节点.

如果在请求时没有设置 `hashKey` 会降级为加权随机.

## hashKey 设置方式

在请求时增加 `grpc.WithHashKey(hashKey string)` 选项发送请求.

## 目标选择

有时候希望能自己决定使用哪个服务节点, 可以在请求时增加 `grpc.WithTarget(serverName string)` 选项发送请求, 该请求会直接选择设置的服务节点, 但是如果该服务节点连接异常会产生 `no instance` 错误.

如果未对服务节点设置服务名, 则这个服务节点的默认服务名的值为该服务的 `host:端口`, 如: `localhost:3000`, `192.168.1.3:3030`

# 注册器

## static

静态注册器. 让客户端主动设置一个或多个服务节点的属性. 如:

+ ```localhost:3000```
+ ```localhost:3000?weight=100```
+ ```localhost:3000?weight=100&name=service1```
+ ```grpc://localhost:3000?weight=100&name=service1```
+ ```grpc://localhost:3001?weight=100&name=service1,grpc://localhost:3002?weight=100&name=service2```

说明

+ `grpc://` 表示协议类型, 如果未设置默认为 `grpc://`.
+ `localhost:3000` 为服务节点地址, 必须设置.
+ `weight` 为该节点设置权重值, 如果未设置默认为 `100`.
+ `name` 为该节点设置服务名, 如果未设置默认为服务节点地址

