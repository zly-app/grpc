<!-- TOC -->

- [grpc客户端组件](#grpc客户端组件)
- [快速开始](#快速开始)
- [配置文件](#配置文件)

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

	c := grpc.NewGRpcClientCreator(app) // 获取grpc客户端建造者
	// 注册客户端创造者
	c.RegistryGRpcClientCreator("hello", func(cc grpc.ClientConnInterface) interface{} {
		return hello.NewHelloServiceClient(cc)
	})
	helloClient := c.GetGRpcClient("hello").(hello.HelloServiceClient) // 获取客户端

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

```yaml
components:
   grpc:
      hello:
         Address: localhost:3000 # 链接地址
         Registry: static # 注册器, 支持 static
		 Balance: weight_consistent_hash # 均衡器, 支持 round_robin, weight_random, weight_hash, weight_consistent_hash
		 DialTimeout: 5 # 连接超时, 单位秒
		 InsecureDial: true # 是否启用不安全的连接, 如果没有设置tls必须开启
		 EnableOpenTrace: true # 是否启用OpenTrace
		 ReqLogLevelIsInfo: true # 是否将请求日志等级设为info
		 ConnPoolSize: 5 # conn池大小, 表示对每个服务节点最少开启多少个链接
		 MaxConnPoolSize: 20 # conn池最大大小, 表示对每个服务节点最多开启多少个链接
		 AcquireIncrement: 5 # 当连接池中的连接耗尽的时候一次同时获取的连接数
		 ConnIdleTime: 60 # conn空闲时间, 单位秒, 当conn空闲达到一定时间则被标记为可释放
		 AutoReleaseConnInterval: 10 # 自动释放空闲conn检查间隔时间, 单位秒
		 MaxWaitConnSize: 1000 # 最大等待conn数量, 当连接池满后, 新建连接将等待池中连接释放后才可以继续, 等待的数量超出阈值则返回错误
		 WaitConnTime: 5 # 等待conn时间, 单位秒, 表示在conn池中获取一个conn的最大等待时间, -1表示一直等待直到有可用池
		 ProxyAddress: "" # 代理地址. 支持 socks5, socks5h. 示例: socks5://127.0.0.1:1080
		 ProxyUser: "" # 代理用户名
		 ProxyPasswd: "" # 代理用户密码
```
