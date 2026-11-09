// Package resolver 是底座 —— gRPC Resolver 的地址广播机制。
//
// 拥有：无业务状态，只有「当前地址集 + 已连接的 ClientConn 集合」。
// 提供：NewBuilderWithScheme / InitialState / UpdateState / Build。
// 允许调用：无上层。终止保障：UpdateState 只单向推状态、不回灌、不触发新的发现请求；
// CCs 集合由互斥锁保护，Close 时移除引用，广播不会无限递归。
package resolver

import (
	"sync"

	"google.golang.org/grpc/resolver"
)

// Builder 同时是 resolver.Builder 和状态广播入口。
type Builder struct {
	scheme         string
	ccs            map[resolver.ClientConn]struct{}
	bootstrapState *resolver.State

	mx sync.RWMutex
}

// NewBuilderWithScheme 建一个指定 scheme 的广播器。
func NewBuilderWithScheme(scheme string) *Builder {
	return &Builder{
		scheme: scheme,
		ccs:    make(map[resolver.ClientConn]struct{}),
	}
}

// InitialState 设置首次连接的地址集。
func (r *Builder) InitialState(s resolver.State) {
	r.mx.Lock()
	defer r.mx.Unlock()
	r.bootstrapState = &s
}

// Build 实现 resolver.Builder。
func (r *Builder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.ccs[cc] = struct{}{}
	if r.bootstrapState != nil {
		_ = cc.UpdateState(*r.bootstrapState)
	}
	return &ccWrapper{Builder: r, cc: cc}, nil
}

// Scheme 实现 resolver.Builder。
func (r *Builder) Scheme() string { return r.scheme }

// ResolveNow 实现 resolver.Resolver；状态由模型主动推，这里无需动作。
func (r *Builder) ResolveNow(resolver.ResolveNowOptions) {}

// UpdateState 向所有已连接的 ClientConn 广播新地址集。
func (r *Builder) UpdateState(s resolver.State) {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.bootstrapState = &s
	for cc := range r.ccs {
		_ = cc.UpdateState(s)
	}
}

func (r *Builder) removeCC(cc resolver.ClientConn) {
	r.mx.Lock()
	defer r.mx.Unlock()
	delete(r.ccs, cc)
}

type ccWrapper struct {
	*Builder
	cc resolver.ClientConn
}

func (w *ccWrapper) Close() { w.Builder.removeCC(w.cc) }

var _ resolver.Builder = (*Builder)(nil)
