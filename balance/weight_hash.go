package balance

import (
	"github.com/zlyuancn/zbalancer"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"

	"github.com/zly-app/grpc/pkg"
)

const (
	/*加权hash
	  每个实例有不同的权重, 获取时根据提供的key计算hash值然后对总权重求余, 余数计算所在实例, 权重越高被选取的机会越大.
	  如果没有设置key则降级为加权随机.
	*/
	WeightHash = "weight_hash"
)

func init() {
	b := base.NewBalancerBuilder(
		WeightHash,
		&basePickerBuilder{
			BalancerType: zbalancer.WeightHashBalancer,
			PickerCreator: func(b zbalancer.Balancer) balancer.Picker {
				return &whPicker{Balancer: b}
			},
		},
		base.Config{HealthCheck: true},
	)
	RegistryBalancerBuilder(WeightHash, b)
}

type whPicker struct {
	zbalancer.Balancer
}

func (p *whPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	target := pkg.GetTargetByCtx(info.Ctx)
	hashKey := pkg.GetHashKeyByCtx(info.Ctx)
	ins, err := p.Get(zbalancer.WithTarget(target), zbalancer.WithHashKey(hashKey))
	if err != nil {
		return balancer.PickResult{}, err
	}

	return balancer.PickResult{SubConn: ins.Instance().(balancer.SubConn)}, nil
}
