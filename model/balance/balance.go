// Package balance 是模型层 —— 均衡策略表与选点规则的所有者。
//
// 拥有：策略名 → 策略实现的只读表、Picker 的选点规则、balancer 状态机。
// 提供：New / Install / DialOption（port.Balancer）。
// 允许调用：foundation/addr（权重）、foundation/ctxkey（target / hashKey）。
// 禁止：注册侧/发现侧直接读本包内部表；横向调用只走 port.Balancer。
package balance

import (
	"fmt"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"

	"github.com/zly-app/grpc/foundation/port"
)

// 策略名一律带 zly_ 前缀：gRPC 的 balancer 注册表是进程级全局表，
// round_robin 等名字已被 grpc-go 内置均衡器占用，同名注册会静默覆盖掉
// 进程内所有其它 gRPC 客户端的实现。
const (
	RoundRobin           = "zly_round_robin"
	WeightRandom         = "zly_weight_random"
	WeightHash           = "zly_weight_hash"
	WeightConsistentHash = "zly_weight_consistent_hash"
	LeastRequest         = "zly_least_request"
)

// Default 是客户端未配置 Balance 时使用的策略。
const Default = LeastRequest

type spec struct {
	newStrategy func() strategy
}

// specs 是只读策略表；用函数构造而非包级可变 map，避免全局状态。
func specs() map[string]spec {
	return map[string]spec{
		RoundRobin:           {newStrategy: func() strategy { return new(roundStrategy) }},
		WeightRandom:         {newStrategy: func() strategy { return new(weightRandomStrategy) }},
		WeightHash:           {newStrategy: func() strategy { return new(weightHashStrategy) }},
		WeightConsistentHash: {newStrategy: func() strategy { return new(ringStrategy) }},
		LeastRequest:         {newStrategy: func() strategy { return new(leastRequestStrategy) }},
	}
}

// Table 是均衡策略模型实例（无可变状态，装配动作由 Install 完成）。
type Table struct{}

// New 建一张策略表。
func New() *Table { return &Table{} }

var installOnce sync.Once

// Install 把全部策略显式注册进 gRPC 的全局 balancer 表。
// gRPC 的注册表是进程级框架契约，once 只作幂等守卫。
func (t *Table) Install() {
	installOnce.Do(func() {
		for name, s := range specs() {
			balancer.Register(&builder{name: name, newStrategy: s.newStrategy})
		}
	})
}

// legacyNames 是 v0.7.0 之前的策略名，只用于报错时给出迁移目标，不做自动映射——
// 静默改名会让「我想要一致性 hash」和「我拿到一致性 hash」脱钩。
var legacyNames = map[string]string{
	"round_robin":            RoundRobin,
	"weight_random":          WeightRandom,
	"weight_hash":            WeightHash,
	"weight_consistent_hash": WeightConsistentHash,
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// DialOption 把策略名翻译成 gRPC 拨号选项；未知名字显式报错。
func (t *Table) DialOption(name string) (grpc.DialOption, error) {
	name = normalizeName(name)
	if _, ok := specs()[name]; !ok {
		if target, ok := legacyNames[name]; ok {
			return nil, fmt.Errorf("balancer 已改名: %q → %q（v0.7.0 起策略名统一加 zly_ 前缀）", name, target)
		}
		return nil, fmt.Errorf("balancer 不存在: %v", name)
	}
	return grpc.WithDefaultServiceConfig(fmt.Sprintf(`{ "LoadBalancingConfig": [{"%s": {}}] }`, name)), nil
}

var _ port.Balancer = (*Table)(nil)
