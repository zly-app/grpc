package static

import (
	"context"
	"fmt"
	"sync"

	"github.com/zly-app/zapp/component/conn"
	"github.com/zly-app/zapp/core"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	"github.com/zly-app/grpc/pkg"
	"github.com/zly-app/grpc/registry"
)

const Name = "static"

func init() {
	registry.AddCreator(Name, NewManual)
}

type StaticRegistry struct {
	res     map[string]resolver.Builder
	address map[string][]*pkg.AddrInfo
	mx      sync.RWMutex

	conn *conn.Conn
}

var DefStatic = newStatic()

func newStatic() *StaticRegistry {
	s := &StaticRegistry{
		res:     make(map[string]resolver.Builder),
		address: make(map[string][]*pkg.AddrInfo),
		conn:    conn.NewConn(),
	}
	return s
}

func (s *StaticRegistry) Close() {}

func (s *StaticRegistry) GetBuilder(ctx context.Context, serverName string) (resolver.Builder, error) {
	s.mx.RLock()
	b, ok := s.res[serverName]
	s.mx.RUnlock()

	if ok {
		return b, nil
	}

	s.mx.Lock()
	defer s.mx.Unlock()
	b, ok = s.res[serverName]
	if ok {
		return b, nil
	}

	address, ok := s.address[serverName]
	if !ok || len(address) == 0 {
		return nil, fmt.Errorf("%s address is empty", serverName)
	}
	r := manual.NewBuilderWithScheme(Name)
	addrList := make([]resolver.Address, len(address))
	for i, a := range address {
		addr := resolver.Address{Addr: a.Endpoint}
		addr = pkg.SetAddrInfo(addr, a)
		addrList[i] = addr
	}
	r.InitialState(resolver.State{Addresses: addrList})

	s.res[serverName] = r
	return r, nil

}
func (s *StaticRegistry) Registry(ctx context.Context, serverName string, addr *pkg.AddrInfo) error {
	s.mx.Lock()
	defer s.mx.Unlock()

	delete(s.res, serverName)
	s.address[serverName] = append(s.address[serverName], addr)
	return nil
}
func (s *StaticRegistry) UnRegistry(ctx context.Context, serverName string) {
	s.mx.Lock()
	defer s.mx.Unlock()

	delete(s.address, serverName)
}

// 创建Manual
func NewManual(_ core.IApp, _ string) (registry.Registry, error) {
	return DefStatic, nil
}
