package registry

import (
	"fmt"
	"strings"

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

// 分隔端点和参数, addr示例: localhost:3000?k=v&k2=v2
func SplitParams(addr string) (endpoint string, params map[string]string) {
	params = make(map[string]string)
	k := strings.Index(addr, "?")
	if k == -1 {
		return addr, params
	}

	endpoint, paramText := addr[:k], addr[k+1:]
	if paramText == "" {
		return endpoint, params
	}

	args := strings.Split(paramText, "&")
	for _, arg := range args {
		if arg == "" {
			continue
		}
		kvs := strings.SplitN(arg, "=", 2)
		if len(kvs) == 1 {
			params[kvs[0]] = ""
			continue
		}
		params[kvs[0]] = kvs[1]
	}
	return endpoint, params
}
