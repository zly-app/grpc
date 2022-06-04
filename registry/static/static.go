package static

import (
	"errors"
	"strings"
	"sync"

	"go.uber.org/zap"
	"google.golang.org/grpc/resolver"

	"github.com/zly-app/zapp/logger"
)

const (
	Name = "static"
)

var staticRegistry = newStaticRegistry()

// 注册地址
func RegistryAddress(serverName, address string) {
	staticRegistry.RegistryEndpoint(serverName, address)
}

type StaticRegistry struct {
	endpoints map[string][]resolver.Address
	mx        sync.RWMutex
}

func newStaticRegistry() *StaticRegistry {
	r := &StaticRegistry{
		endpoints: make(map[string][]resolver.Address),
	}
	resolver.Register(r)
	return r
}

func (s *StaticRegistry) RegistryEndpoint(serviceName, address string) {
	if address == "" {
		logger.Log.Fatal("endpoint is empty", zap.String("name", serviceName))
	}

	ss := strings.Split(address, ",")
	addresses := make([]resolver.Address, len(ss))
	for i, s := range ss {
		addresses[i] = resolver.Address{Addr: s}
	}

	s.mx.Lock()
	s.endpoints[serviceName] = addresses
	s.mx.Unlock()
}

func (s *StaticRegistry) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	s.mx.RLock()
	name := strings.TrimLeft(target.URL.Path, "/")
	address := s.endpoints[name]
	s.mx.RUnlock()
	if len(address) == 0 {
		return nil, errors.New("address of endpoint is empty or unregistered")
	}

	err := cc.UpdateState(resolver.State{Addresses: address})
	if err != nil {
		return nil, err
	}
	return s, err
}
func (s *StaticRegistry) Scheme() string { return Name }

func (s *StaticRegistry) ResolveNow(options resolver.ResolveNowOptions) {}
func (s *StaticRegistry) Close()                                        {}
