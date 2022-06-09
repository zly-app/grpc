package balance

import (
	"github.com/zlyuancn/zbalancer"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"

	"github.com/zly-app/grpc/client/pkg"
)

const (
	RoundRobin = "round_robin"
)

type rrPickerBuilder struct{}

func init() {
	b := base.NewBalancerBuilder(RoundRobin, &rrPickerBuilder{}, base.Config{HealthCheck: true})
	RegistryBalancerBuilder(RoundRobin, b)
}

func (*rrPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	ins := make([]zbalancer.Instance, 0, len(info.ReadySCs))
	for sc, connInfo := range info.ReadySCs {
		addrInfo := pkg.GetAddrInfo(connInfo.Address)
		ins = append(ins, zbalancer.NewInstance(sc).SetName(addrInfo.Name).SetWeight(addrInfo.Weight))
	}
	b, _ := zbalancer.NewBalancer(zbalancer.RoundBalancer)
	b.Update(ins)
	return &rrPicker{
		Balancer: b,
	}
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
