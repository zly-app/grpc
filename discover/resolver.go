package discover

import (
	"sync"

	"google.golang.org/grpc/resolver"
)

func NewBuilderWithScheme(scheme string) *Resolver {
	return &Resolver{
		scheme:         scheme,
		CCs:            make(map[resolver.ClientConn]struct{}),
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
	return r, nil
}
func (r *Resolver) Scheme() string {
	return r.scheme
}
func (r *Resolver) ResolveNow(o resolver.ResolveNowOptions) {}
func (r *Resolver) Close()                                  {}
func (r *Resolver) UpdateState(s resolver.State) {
	r.mx.Lock()
	defer r.mx.Unlock()

	r.bootstrapState = &s
	for cc := range r.CCs {
		_ = cc.UpdateState(s)
	}
}
