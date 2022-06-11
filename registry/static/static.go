package static

import (
	"fmt"
	"net"
	"strings"

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
	*manual.Resolver
}

func (s *StaticRegistry) Registry(addr net.Addr) error { return nil }
func (s *StaticRegistry) UnRegistry() error            { return nil }

// 创建Manual
func NewManual(address string) (registry.Registry, error) {
	if address == "" {
		return nil, fmt.Errorf("address为空")
	}

	r := manual.NewBuilderWithScheme(Name)

	ss := strings.Split(address, ",")
	addrList := make([]resolver.Address, len(ss))
	for i, s := range ss {
		addrInfo, err := pkg.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("解析addr失败: %v", err)
		}
		addr := resolver.Address{Addr: addrInfo.Endpoint}
		addr = pkg.SetAddrInfo(addr, addrInfo)
		addrList[i] = addr
	}

	r.InitialState(resolver.State{Addresses: addrList})
	return &StaticRegistry{r}, nil
}
