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

const Type = "static"

func init() {
	registry.AddCreator(Type, NewManual)
}

type StaticRegistry struct {
	res     map[string]resolver.Builder
	address map[string][]*pkg.AddrInfo
	mx      sync.RWMutex

	conn *conn.AnyConn[resolver.Builder]
}

var DefStatic = newStatic()

func newStatic() *StaticRegistry {
	s := &StaticRegistry{
		res:     make(map[string]resolver.Builder),
		address: make(map[string][]*pkg.AddrInfo),
		conn: conn.NewAnyConn[resolver.Builder](func(name string, conn resolver.Builder) {
			v, ok := conn.(*manual.Resolver)
			if ok {
				v.Close()
			}
		}),
	}
	return s
}

func (s *StaticRegistry) Close() {}

func (s *StaticRegistry) GetBuilder(ctx context.Context, serverName string) (resolver.Builder, error) {
	ins, err := s.conn.GetConn(func(serverName string) (resolver.Builder, error) {
		s.mx.Lock()
		address, ok := s.address[serverName]
		s.mx.Unlock()

		if !ok || len(address) == 0 {
			return nil, fmt.Errorf("%s address is empty", serverName)
		}
		r := manual.NewBuilderWithScheme(Type)
		addrList := make([]resolver.Address, len(address))
		for i, a := range address {
			addr := resolver.Address{Addr: a.Endpoint}
			addr = pkg.SetAddrInfo(addr, a)
			addrList[i] = addr
		}
		r.InitialState(resolver.State{Addresses: addrList})
		return r, nil
	}, serverName)
	if err != nil {
		return nil, err
	}
	return ins, nil
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
