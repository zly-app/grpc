// Package hooks 是模型层 —— 客户端/服务端拦截器注册表的所有者。
//
// 拥有：按服务名的客户端 hook 表 + 全局客户端 hook 表。
// 提供：AddClient / AddAllClient / ClientFor。
// 允许调用：无上层。禁止：靠 BeforeStart 全局钩子做「合并」这种规则动作。
package hooks

import (
	"sync"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
)

// ClientHook 是客户端 unary 拦截器。
type ClientHook = grpc.UnaryClientInterceptor

// ServerHook 是服务端 unary 拦截器。
type ServerHook = grpc.UnaryServerInterceptor

// Table 独占 hook 表，所有装配与读取都经本实例。
type Table struct {
	mx  sync.RWMutex
	per map[string][]ClientHook
	all []ClientHook
}

// New 建一张空 hook 表。
func New() *Table {
	return &Table{per: make(map[string][]ClientHook)}
}

// AddClient 给指定服务追加 hook。
func (t *Table) AddClient(name string, hs ...ClientHook) {
	t.mx.Lock()
	defer t.mx.Unlock()
	t.per[name] = append(t.per[name], hs...)
}

// AddAllClient 给所有服务追加 hook。
func (t *Table) AddAllClient(hs ...ClientHook) {
	t.mx.Lock()
	defer t.mx.Unlock()
	t.all = append(t.all, hs...)
}

// ClientFor 实时合成该服务的拦截器链：全局 hook 在前，服务专属在后。
func (t *Table) ClientFor(name string) ClientHook {
	t.mx.RLock()
	defer t.mx.RUnlock()

	h := make([]ClientHook, 0, len(t.all)+len(t.per[name]))
	h = append(h, t.all...)
	h = append(h, t.per[name]...)
	return grpc_middleware.ChainUnaryClient(h...)
}
