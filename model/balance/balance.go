// Package balance 是模型层 —— 均衡策略表与选点规则的所有者。
//
// 拥有：策略名 → 均衡器类型的只读表、Picker 的选点规则。
// 提供：New / Install / DialOption（port.Balancer）。
// 允许调用：foundation/addr（权重）、foundation/ctxkey（target / hashKey）。
// 禁止：注册侧/发现侧直接读本包内部表；横向调用只走 port.Balancer。
package balance

import (
	"fmt"
	"strings"
	"sync"

	"github.com/zlyuancn/zbalancer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"

	"github.com/zly-app/grpc/foundation/addr"
	"github.com/zly-app/grpc/foundation/ctxkey"
	"github.com/zly-app/grpc/foundation/port"
)

const (
	RoundRobin           = "round_robin"
	WeightRandom         = "weight_random"
	WeightHash           = "weight_hash"
	WeightConsistentHash = "weight_consistent_hash"
)

type spec struct {
	balancerType zbalancer.BalancerType
	useHashKey   bool
}

// specs 是只读策略表；用函数构造而非包级可变 map，避免全局状态。
func specs() map[string]spec {
	return map[string]spec{
		RoundRobin:           {balancerType: zbalancer.RoundBalancer},
		WeightRandom:         {balancerType: zbalancer.WeightRandomBalancer},
		WeightHash:           {balancerType: zbalancer.WeightHashBalancer, useHashKey: true},
		WeightConsistentHash: {balancerType: zbalancer.WeightConsistentHashBalancer, useHashKey: true},
	}
}

// Table 是均衡策略模型实例（无可变状态，装配动作由 Install 完成）。
type Table struct{}

// New 建一张策略表。
func New() *Table { return &Table{} }

var installOnce sync.Once

// Install 把四种策略显式注册进 gRPC 的全局 balancer 表。
// 替代旧版 4 个 init()；gRPC 的注册表是进程级框架契约，once 只作幂等守卫。
func (t *Table) Install() {
	installOnce.Do(func() {
		for name, s := range specs() {
			b := base.NewBalancerBuilder(
				name,
				&pickerBuilder{balancerType: s.balancerType, useHashKey: s.useHashKey},
				base.Config{HealthCheck: true},
			)
			balancer.Register(b)
		}
	})
}

// DialOption 把策略名翻译成 gRPC 拨号选项；未知名字显式报错。
func (t *Table) DialOption(name string) (grpc.DialOption, error) {
	name = strings.ToLower(name)
	if _, ok := specs()[name]; !ok {
		return nil, fmt.Errorf("balancer 不存在: %v", name)
	}
	return grpc.WithDefaultServiceConfig(fmt.Sprintf(`{ "LoadBalancingConfig": [{"%s": ""}] }`, name)), nil
}

var _ port.Balancer = (*Table)(nil)

type pickerBuilder struct {
	balancerType zbalancer.BalancerType
	useHashKey   bool
}

func (b *pickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	ins := make([]zbalancer.Instance, 0, len(info.ReadySCs))
	for sc, connInfo := range info.ReadySCs {
		a := addr.Get(connInfo.Address)
		ins = append(ins, zbalancer.NewInstance(sc).SetName(a.Name).SetWeight(a.Weight))
	}
	impl, _ := zbalancer.NewBalancer(b.balancerType)
	impl.Update(ins)
	return &picker{Balancer: impl, useHashKey: b.useHashKey}
}

type picker struct {
	zbalancer.Balancer
	useHashKey bool
}

func (p *picker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	target := ctxkey.Target(info.Ctx)
	if p.useHashKey {
		ins, err := p.Get(zbalancer.WithTarget(target), zbalancer.WithHashKey(ctxkey.HashKey(info.Ctx)))
		if err != nil {
			return balancer.PickResult{}, err
		}
		return balancer.PickResult{SubConn: ins.Instance().(balancer.SubConn)}, nil
	}
	ins, err := p.Get(zbalancer.WithTarget(target))
	if err != nil {
		return balancer.PickResult{}, err
	}
	return balancer.PickResult{SubConn: ins.Instance().(balancer.SubConn)}, nil
}
