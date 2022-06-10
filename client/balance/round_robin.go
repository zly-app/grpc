package balance

import (
	"github.com/zlyuancn/zbalancer"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"

	"github.com/zly-app/grpc/client/pkg"
)

const (
	/*轮询
	  按顺序获取实例
	*/
	RoundRobin = "round_robin"
)

func init() {
	b := base.NewBalancerBuilder(
		RoundRobin,
		&basePickerBuilder{
			BalancerType: zbalancer.RoundBalancer,
			PickerCreator: func(b zbalancer.Balancer) balancer.Picker {
				return &rrPicker{Balancer: b}
			},
		},
		base.Config{HealthCheck: true},
	)
	RegistryBalancerBuilder(RoundRobin, b)
}

type rrPicker struct {
	zbalancer.Balancer
}

func (p *rrPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	target := pkg.GetTargetByCtx(info.Ctx)
	ins, err := p.Get(zbalancer.WithTarget(target))
	if err != nil {
		return balancer.PickResult{}, err
	}

	return balancer.PickResult{SubConn: ins.Instance().(balancer.SubConn)}, nil
}
