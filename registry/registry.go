package registry

import (
	"context"
	"fmt"

	"github.com/zly-app/zapp/component/conn"
	"google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/pkg"
)

type Registry interface {
	GetBuilder(ctx context.Context, serverName string) (resolver.Builder, error) // 获取builder, 在需要获取client时调用
	Registry(ctx context.Context, serverName string, addr *pkg.AddrInfo) error   // 注册, 在服务端启动成功后会调用这个方法
	UnRegistry(ctx context.Context, serverName string, addr *pkg.AddrInfo)       // 取消注册, 在服务端即将结束前会调用这个方法
	Close()                                                                      // 关闭注册器
}

type RegistryCreator func(address string) (Registry, error)

var registryCreator = map[string]RegistryCreator{}

var registryConn = conn.NewConn()

// 获取注册器
func GetRegistry(registryName, registryAddress string) (Registry, error) {
	ins := registryConn.GetInstance(func(name string) (conn.IInstance, error) {
		creator, ok := registryCreator[name]
		if !ok {
			return nil, fmt.Errorf("注册器不存在: %v", name)
		}
		r, err := creator(registryAddress)
		if err != nil {
			return nil, fmt.Errorf("创建注册器失败: %v", err)
		}
		return r, nil
	}, registryName)
	r := ins.(Registry)
	return r, nil
}

func AddCreator(registryName string, r RegistryCreator) {
	registryCreator[registryName] = r
}
