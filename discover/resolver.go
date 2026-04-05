package discover

import (
	"sync"

	"google.golang.org/grpc/resolver"
)

func NewBuilderWithScheme(scheme string) *Resolver {
	return &Resolver{
		scheme: scheme,
		CCs:    make(map[resolver.ClientConn]struct{}),
	}
}

type Resolver struct {
	scheme         string
	CCs            map[resolver.ClientConn]struct{}
	bootstrapState *resolver.State

	mx sync.RWMutex
}

func (r *Resolver) InitialState(s resolver.State) {
	r.bootstrapState = &s
}

func (r *Resolver) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.CCs[cc] = struct{}{}
	if r.bootstrapState != nil {
		_ = cc.UpdateState(*r.bootstrapState)
	}
	return &ccWrapper{Resolver: r, cc: cc}, nil
}

func (r *Resolver) Scheme() string {
	return r.scheme
}

func (r *Resolver) ResolveNow(o resolver.ResolveNowOptions) {}

func (r *Resolver) UpdateState(s resolver.State) {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.bootstrapState = &s
	for cc := range r.CCs {
		_ = cc.UpdateState(s)
	}
}

func (r *Resolver) removeCC(cc resolver.ClientConn) {
	r.mx.Lock()
	defer r.mx.Unlock()
	delete(r.CCs, cc)
}

// ccWrapper 包装 resolver，在 Close 时清理 CCs 中的引用
type ccWrapper struct {
	*Resolver
	cc resolver.ClientConn
}

func (w *ccWrapper) Close() {
	w.Resolver.removeCC(w.cc)
}
