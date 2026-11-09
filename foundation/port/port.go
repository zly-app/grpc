// Package port 是底座 —— 模型/用例之间横向调用的唯一合法通道（窄接口）。
//
// 拥有：无。提供：一组窄接口。允许调用：无上层。
// 接口定义在使用方之外，是为了让组合根能把具体实现注入给任何一方，
// 而不产生「实现包互相 import」的反向边。
package port

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/foundation/addr"
)

// Registry 是注册中心的 port：状态变更与通知必须由实现方在同一入口内完成。
type Registry interface {
	Register(ctx context.Context, serverName string, a *addr.Info) error
	Unregister(ctx context.Context, serverName string)
	Close()
}

// StaticTable 是进程内静态地址表的 port。
// 注册侧与发现侧由组合根注入同一个实例，避免读侧反向依赖写侧。
type StaticTable interface {
	Registry
	Builder(ctx context.Context, serverName string) (resolver.Builder, error)
}

// Discover 是服务发现的 port。
type Discover interface {
	Builder(ctx context.Context, serverName string) (resolver.Builder, error)
	Close()
}

// Balancer 是负载均衡策略表的 port。
type Balancer interface {
	DialOption(name string) (grpc.DialOption, error)
}

// ServerRunner 是 gRPC 服务端 transport 的 port，由接口层实现、业务层调度。
// BoundPort 只在 Start 成功后有效，供注册用例构造公告地址。
type ServerRunner interface {
	RegisterService(desc *grpc.ServiceDesc, impl interface{})
	Start() error
	BoundPort() int
	Close()
}

// Conn 是一条可关闭的 gRPC 客户端连接。
type Conn interface {
	grpc.ClientConnInterface
	Close() error
}
