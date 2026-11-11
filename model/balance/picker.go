package balance

import (
	"errors"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zly-app/grpc/foundation/ctxkey"
)

// picker 是一次状态快照：strategy 长生命周期，byName 只含 Ready 实例。
type picker struct {
	strategy strategy
	byName   map[string]*instance
}

func (p *picker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	// 点名实例：语义是「就发给它」，实例不在 Ready 集合时明确失败，
	// 而不是退化成随便选一个（那会让调用方拿到错实例的数据）。
	if target := ctxkey.Target(info.Ctx); target != "" {
		inst := p.byName[target]
		if inst == nil {
			return balancer.PickResult{}, status.Errorf(codes.Unavailable, "目标实例不可用: %s", target)
		}
		return picked(inst), nil
	}

	inst, err := p.strategy.Pick(ctxkey.HashKey(info.Ctx))
	if err != nil {
		return balancer.PickResult{}, pickError(err)
	}
	return picked(inst), nil
}

// picked 统一登记在途计数并挂 Done 回调：gRPC 在 RPC 结束时（无论成败）回调它，
// 用来回收计数、为 P2C 提供观测；以后加 EWMA 延迟权重也从这里挂。
func picked(inst *instance) balancer.PickResult {
	inst.inflight.Add(1)
	return balancer.PickResult{SubConn: inst.sc, Done: inst.done}
}

// pickError 把「没有实例」翻译成 gRPC 的等待语义。
// gRPC 只对 balancer.ErrNoSubConnAvailable 阻塞等新 picker，其余错误 failfast 结束 RPC。
func pickError(err error) error {
	if errors.Is(err, errNoInstance) {
		return balancer.ErrNoSubConnAvailable
	}
	return err
}
