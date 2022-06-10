package balance

import (
	"github.com/zlyuancn/zbalancer"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"

	"github.com/zly-app/grpc/client/pkg"
)

const (
	/*加权随机
	  每个实例有不同权重, 获取时随机选择一个实例, 权重越高被选取的机会越大.
	*/
	WeightRandom = "weight_random"
)

func init() {
	b := base.NewBalancerBuilder(
		WeightRandom,
		&basePickerBuilder{
			BalancerType: zbalancer.WeightRandomBalancer,
			PickerCreator: func(b zbalancer.Balancer) balancer.Picker {
				return &wrPicker{Balancer: b}
			},
		},
		base.Config{HealthCheck: true},
	)
	RegistryBalancerBuilder(WeightRandom, b)
}

type wrPicker struct {
	zbalancer.Balancer
}

func (p *wrPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	target := pkg.GetTargetByCtx(info.Ctx)
	ins, err := p.Get(zbalancer.WithTarget(target))
	if err != nil {
		return balancer.PickResult{}, err
	}

	return balancer.PickResult{SubConn: ins.Instance().(balancer.SubConn)}, nil
}
