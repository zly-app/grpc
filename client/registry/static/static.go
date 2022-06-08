package static

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/grpc/balancer/weightedroundrobin"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	"github.com/zly-app/grpc/client/registry"
)

const Name = "static"

const WeightField = "weight"

func init() {
	registry.AddCreator(Name, NewManual)
}

/*创建Manual
  address 地址, 它应该是这样的: localhost:3000,localhost:3001?weight=100
  如果未设置 weight, 默认为 100
*/
func NewManual(address string) (registry.Registry, error) {
	if address == "" {
		return nil, fmt.Errorf("address为空")
	}

	r := manual.NewBuilderWithScheme(Name)

	ss := strings.Split(address, ",")
	addrList := make([]resolver.Address, len(ss))
	for i, s := range ss {
		endpoint, weight := SplitWight(s)
		addr := resolver.Address{Addr: endpoint}
		addr = weightedroundrobin.SetAddrInfo(addr, weightedroundrobin.AddrInfo{Weight: weight})
		addrList[i] = addr
	}

	r.InitialState(resolver.State{Addresses: addrList})
	return r, nil
}

// 分隔权重
func SplitWight(addr string) (endpoint string, weight uint32) {
	weight = 100
	endpoint, params := registry.SplitParams(addr)
	weightText, ok := params[WeightField]
	if !ok {
		return endpoint, weight
	}
	if v, err := strconv.Atoi(weightText); err == nil {
		weight = uint32(v)
	}
	return endpoint, weight
}
