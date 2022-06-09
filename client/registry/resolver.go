package registry

import (
	"fmt"

	"google.golang.org/grpc/resolver"
)

type Registry = resolver.Builder

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
