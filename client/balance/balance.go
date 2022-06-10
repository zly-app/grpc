package balance

import (
	"fmt"

	"github.com/zlyuancn/zbalancer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"

	"github.com/zly-app/grpc/client/pkg"
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

type basePickerBuilder struct {
	BalancerType  zbalancer.BalancerType
	PickerCreator func(b zbalancer.Balancer) balancer.Picker
}

func (b *basePickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	ins := make([]zbalancer.Instance, 0, len(info.ReadySCs))
	for sc, connInfo := range info.ReadySCs {
		addrInfo := pkg.GetAddrInfo(connInfo.Address)
		ins = append(ins, zbalancer.NewInstance(sc).SetName(addrInfo.Name).SetWeight(addrInfo.Weight))
	}
	bImpl, _ := zbalancer.NewBalancer(b.BalancerType)
	bImpl.Update(ins)
	return b.PickerCreator(bImpl)
}
