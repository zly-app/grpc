// Package connreg 是模型层 —— 客户端连接集合的所有者。
//
// 拥有：name → port.Conn 的映射、懒创建器。
// 提供：Get（同名只创建一次）、CloseAll（幂等）。
// 允许调用：foundation/port.Conn。禁止：外部直接持有或替换底层连接。
package connreg

import (
	"sync"

	"github.com/zly-app/grpc/foundation/port"
)

// Maker 由组合根注入，负责真正建一条连接。
type Maker func(name string) (port.Conn, error)

// Collection 独占连接集合。
type Collection struct {
	mx    sync.Mutex
	conns map[string]port.Conn
	make  Maker
}

// New 建一个连接集合。
func New(maker Maker) *Collection {
	return &Collection{conns: make(map[string]port.Conn), make: maker}
}

// Get 取连接；创建失败不缓存，允许下次重试。
func (c *Collection) Get(name string) (port.Conn, error) {
	c.mx.Lock()
	defer c.mx.Unlock()

	if v, ok := c.conns[name]; ok {
		return v, nil
	}
	v, err := c.make(name)
	if err != nil {
		return nil, err
	}
	c.conns[name] = v
	return v, nil
}

// CloseAll 关闭并清空全部连接，可重复调用。
func (c *Collection) CloseAll() {
	c.mx.Lock()
	defer c.mx.Unlock()
	for name, v := range c.conns {
		_ = v.Close()
		delete(c.conns, name)
	}
}
