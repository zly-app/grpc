package registry

import (
	"fmt"
	"net"

	"google.golang.org/grpc/resolver"
)

type Registry interface {
	resolver.Builder
	Registry(addr net.Addr) error // 注册
	UnRegistry() error            // 取消注册
}

type RegistryCreator func(address string) (Registry, error)

var registryCreator = map[string]RegistryCreator{}

// 获取注册器
func GetRegistry(name, address string) (Registry, error) {
	creator, ok := registryCreator[name]
	if !ok {
		return nil, fmt.Errorf("注册器不存在: %v", name)
	}
	r, err := creator(address)
	if err != nil {
		return nil, fmt.Errorf("创建注册器失败: %v", err)
	}
	return r, nil
}

func AddCreator(name string, r RegistryCreator) {
	registryCreator[name] = r
}
