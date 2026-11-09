// Package serverreg 是模型层 —— 服务声明表与重名规则的所有者。
//
// 拥有：serverName → Entry（hooks + 已绑定的 pb 服务）、重名判定。
// 提供：Declare（唯一声明入口）、Entries。
// 允许调用：无上层。禁止：持有 transport 对象、读取配置、触发生命周期钩子。
package serverreg

import (
	"fmt"
	"sync"

	"google.golang.org/grpc"
)

// Binding 是一次 pb 服务注册。
type Binding struct {
	Desc *grpc.ServiceDesc
	Impl interface{}
}

// Entry 是一个 gRPC server 的全部声明。
type Entry struct {
	Name     string
	Hooks    []grpc.UnaryServerInterceptor
	Bindings []Binding
}

// Table 独占声明表。
type Table struct {
	mx          sync.Mutex
	globalHooks []grpc.UnaryServerInterceptor
	declared    map[string]bool
	entries     map[string]*Entry
}

// New 建一张声明表；globalHooks 会并入每个服务的 hook 链。
func New(globalHooks ...grpc.UnaryServerInterceptor) *Table {
	return &Table{
		globalHooks: append([]grpc.UnaryServerInterceptor(nil), globalHooks...),
		declared:    make(map[string]bool),
		entries:     make(map[string]*Entry),
	}
}

// Declare 声明一个具名 server；同名重复声明显式报错。
// 返回的 registrar 可注册多个 pb 服务，它们复用同一个 transport。
func (t *Table) Declare(name string, hs ...grpc.UnaryServerInterceptor) (grpc.ServiceRegistrar, error) {
	t.mx.Lock()
	defer t.mx.Unlock()
	if name != "" {
		if t.declared[name] {
			return nil, fmt.Errorf("grpc server name 重复: %s", name)
		}
		t.declared[name] = true
	}
	return &registrar{table: t, name: name, hooks: append([]grpc.UnaryServerInterceptor(nil), hs...)}, nil
}

// Entries 返回声明快照，供启动用例遍历。
func (t *Table) Entries() []*Entry {
	t.mx.Lock()
	defer t.mx.Unlock()
	out := make([]*Entry, 0, len(t.entries))
	for _, e := range t.entries {
		copy := *e
		copy.Bindings = append([]Binding(nil), e.Bindings...)
		out = append(out, &copy)
	}
	return out
}

func (t *Table) bind(name string, hs []grpc.UnaryServerInterceptor, desc *grpc.ServiceDesc, impl interface{}) {
	t.mx.Lock()
	defer t.mx.Unlock()

	e, ok := t.entries[name]
	if !ok {
		h := make([]grpc.UnaryServerInterceptor, 0, len(t.globalHooks)+len(hs))
		h = append(h, t.globalHooks...)
		h = append(h, hs...)
		e = &Entry{Name: name, Hooks: h}
		t.entries[name] = e
	}
	e.Bindings = append(e.Bindings, Binding{Desc: desc, Impl: impl})
}

type registrar struct {
	table *Table
	name  string
	hooks []grpc.UnaryServerInterceptor
}

// RegisterService 实现 grpc.ServiceRegistrar；未命名时以 desc.ServiceName 为准。
func (r *registrar) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	name := r.name
	if name == "" && desc != nil {
		name = desc.ServiceName
	}
	r.table.bind(name, r.hooks, desc, impl)
}
