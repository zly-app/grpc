// Package hashkey 是模型层 —— 网关一致性 hash key 规则的唯一所有者。
//
// 拥有：path → hashKey 请求头的规则表与默认来源。
// 提供：New / HeaderFor。
// 允许调用：model/config（HashKeyConfig / HashKeyRule 配置值对象）。
// 禁止：读取网关单例的私有配置、读取请求头；规则只能经本模型查询。
package hashkey

import "github.com/zly-app/grpc/model/config"

// Table 独占 hashKey 规则表；构造后只读，不需要锁。
type Table struct {
	defaultHeader string
	headerByPath  map[string]string // 值为空表示该 path 显式不参与 hash
}

// New 从配置值对象建表；规则合法性由 config.Check 负责，这里只做空 Path 防御。
func New(conf config.HashKeyConfig) *Table {
	t := &Table{
		defaultHeader: conf.ByHeader,
		headerByPath:  make(map[string]string, len(conf.Rules)),
	}
	for _, r := range conf.Rules {
		if r.Path == "" {
			continue
		}
		t.headerByPath[r.Path] = r.ByHeader
	}
	return t
}

// HeaderFor 返回该 path 应使用的 hashKey 请求头。
//
// 命中规则时以规则为准（规则头名为空表示显式不参与 hash，不回落默认）；
// 未命中规则时使用默认来源。第二个返回值为 false 表示不注入 hashKey。
func (t *Table) HeaderFor(path string) (string, bool) {
	if h, ok := t.headerByPath[path]; ok {
		return h, h != ""
	}
	return t.defaultHeader, t.defaultHeader != ""
}
