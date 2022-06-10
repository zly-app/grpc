package balance

import (
	"github.com/zlyuancn/zbalancer"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"

	"github.com/zly-app/grpc/client/pkg"
)

const (
	/*加权一致性hash
	  每个实例有不同的权重, 权重值可以理解为每个实例的分片数, 每个分片计算hash值落在一个环上. 获取时根据提供的key计算hash值然后得出落在环的一个点上, 由这个点得出是哪个实例的分片进而知道是哪个实例.
	  如果没有设置key则降级为加权随机.
	*/
	WeightConsistentHash = "weight_consistent_hash"
)

func init() {
	b := base.NewBalancerBuilder(
		WeightConsistentHash,
		&basePickerBuilder{
			BalancerType: zbalancer.WeightConsistentHashBalancer,
			PickerCreator: func(b zbalancer.Balancer) balancer.Picker {
				return &wchPicker{Balancer: b}
			},
		},
		base.Config{HealthCheck: true},
	)
	RegistryBalancerBuilder(WeightConsistentHash, b)
}

type wchPicker struct {
	zbalancer.Balancer
}

func (p *wchPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	target := pkg.GetTargetByCtx(info.Ctx)
	hashKey := pkg.GetHashKeyByCtx(info.Ctx)
	ins, err := p.Get(zbalancer.WithTarget(target), zbalancer.WithHashKey(hashKey))
	if err != nil {
		return balancer.PickResult{}, err
	}

	return balancer.PickResult{SubConn: ins.Instance().(balancer.SubConn)}, nil
}
