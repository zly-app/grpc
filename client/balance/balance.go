package balance

import (
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
)

var balancerBuilders = map[string]struct{}{}

// 获取均衡器连接选项
func GetBalanceDialOption(name string) (grpc.DialOption, error) {
	_, ok := balancerBuilders[name]
	if !ok {
		return nil, fmt.Errorf("balancer 不存在: %v", name)
	}
	format := `{ "LoadBalancingConfig": [{"%s": ""}] }`
	return grpc.WithDefaultServiceConfig(fmt.Sprintf(format, name)), nil
}

// 注册均衡器构建器
func RegistryBalancerBuilder(name string, b balancer.Builder) {
	balancerBuilders[name] = struct{}{}
	balancer.Register(b)
}
