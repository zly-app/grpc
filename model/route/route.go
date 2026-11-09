// Package route 是模型层 —— 网关路由规则的所有者。
//
// 拥有：path → hashKey header 的路由表。
// 提供：New / HeaderFor。
// 允许调用：model/config.RouteConfig（配置值对象）。
// 禁止：读取网关单例的私有配置；规则只能经本模型查询。
package route

import "github.com/zly-app/grpc/model/config"

// Table 独占路由表；构造后只读，不需要锁。
type Table struct {
	headerByPath map[string]string
}

// New 从配置项构建路由表；同一 path 以最后一条为准。
func New(items []config.RouteConfig) *Table {
	t := &Table{headerByPath: make(map[string]string, len(items))}
	for _, r := range items {
		if r.Path == "" {
			continue
		}
		t.headerByPath[r.Path] = r.HashKeyByHeader
	}
	return t
}

// HeaderFor 返回该 path 应用哪个 header 作为 hashKey；无规则或规则为空时返回 false。
func (t *Table) HeaderFor(path string) (string, bool) {
	h, ok := t.headerByPath[path]
	return h, ok && h != ""
}
