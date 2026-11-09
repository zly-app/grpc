package registry

import (
	"context"
	"fmt"
	"sync"

	gresolver "google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/foundation/addr"
	"github.com/zly-app/grpc/foundation/port"
	fresolver "github.com/zly-app/grpc/foundation/resolver"
)

// StaticTable 是进程内静态地址表，由组合根创建唯一实例并同时注入注册侧与发现侧。
// 数据独占：address 与 builders 只由本表读写。
type StaticTable struct {
	mx       sync.RWMutex
	address  map[string][]*addr.Info
	builders map[string]*fresolver.Builder
}

// NewStaticTable 建一张空地址表。
func NewStaticTable() *StaticTable {
	return &StaticTable{
		address:  make(map[string][]*addr.Info),
		builders: make(map[string]*fresolver.Builder),
	}
}

// Register 是唯一施放入口：写地址与广播状态在同一次调用里完成。
func (s *StaticTable) Register(_ context.Context, serverName string, a *addr.Info) error {
	if a == nil || a.Endpoint == "" {
		return fmt.Errorf("静态注册地址为空: %s", serverName)
	}

	s.mx.Lock()
	s.address[serverName] = append(s.address[serverName], a)
	b, ok := s.builders[serverName]
	if !ok {
		b = fresolver.NewBuilderWithScheme(TypeStatic)
		s.builders[serverName] = b
	}
	state := s.stateOf(serverName)
	s.mx.Unlock()

	b.InitialState(state)
	b.UpdateState(state) // 已连接的 client 立刻看到新地址，无需等下次拨号
	return nil
}

// Unregister 是唯一施放入口：删地址并广播空状态。
func (s *StaticTable) Unregister(_ context.Context, serverName string) {
	s.mx.Lock()
	delete(s.address, serverName)
	b := s.builders[serverName]
	s.mx.Unlock()

	if b != nil {
		b.UpdateState(gresolver.State{})
	}
}

// Builder 返回该服务的 resolver.Builder；地址为空时显式报错。
func (s *StaticTable) Builder(_ context.Context, serverName string) (gresolver.Builder, error) {
	s.mx.Lock()
	b, ok := s.builders[serverName]
	if !ok {
		b = fresolver.NewBuilderWithScheme(TypeStatic)
		s.builders[serverName] = b
	}
	state := s.stateOf(serverName)
	s.mx.Unlock()

	if len(state.Addresses) == 0 {
		return nil, fmt.Errorf("%s address is empty", serverName)
	}
	b.InitialState(state)
	return b, nil
}

// Close 清空地址表与广播器引用。
func (s *StaticTable) Close() {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.address = make(map[string][]*addr.Info)
	s.builders = make(map[string]*fresolver.Builder)
}

func (s *StaticTable) stateOf(serverName string) gresolver.State {
	list := s.address[serverName]
	addresses := make([]gresolver.Address, 0, len(list))
	for _, a := range list {
		ra := gresolver.Address{Addr: a.Endpoint}
		ra = addr.Set(ra, a)
		addresses = append(addresses, ra)
	}
	return gresolver.State{Addresses: addresses}
}

var _ port.StaticTable = (*StaticTable)(nil)
