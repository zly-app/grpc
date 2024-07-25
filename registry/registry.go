package registry

import (
	"context"
	"fmt"

	"github.com/zly-app/zapp/component/conn"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/handler"

	"github.com/zly-app/grpc/pkg"
)

func init() {
	handler.AddHandler(handler.AfterExitHandler, func(app core.IApp, handlerType handler.HandlerType) {
		registryConn.CloseAll()
	})
}

type Registry interface {
	Registry(ctx context.Context, serverName string, addr *pkg.AddrInfo) error // 注册, 在服务端启动成功后会调用这个方法
	UnRegistry(ctx context.Context, serverName string)                         // 取消注册, 在服务端即将结束前会调用这个方法
	Close()                                                                    // 关闭注册器
}

type RegistryCreator func(app core.IApp, address string) (Registry, error)

var registryCreator = map[string]RegistryCreator{}

var registryConn = conn.NewConn()

func GetRegistry(app core.IApp, registryType, registryName string) (Registry, error) {
	key := registryType + "/" + registryName
	ins, err := registryConn.GetConn(func(key string) (conn.IInstance, error) {
		creator, ok := registryCreator[registryType]
		if !ok {
			return nil, fmt.Errorf("注册器不存在: %v", registryType)
		}
		r, err := creator(app, registryName)
		if err != nil {
			return nil, fmt.Errorf("创建注册器失败: %v", err)
		}
		return r, nil
	}, key)
	if err != nil {
		return nil, err
	}
	r := ins.(Registry)
	return r, nil
}

func AddCreator(registryName string, r RegistryCreator) {
	registryCreator[registryName] = r
}
