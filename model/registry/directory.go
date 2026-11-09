package registry

import (
	"context"
	"fmt"
	"sync"

	"github.com/zly-app/grpc/foundation/port"
)

// Directory 独占注册器实例池：type/addr → port.Registry。
// 组合根把同一个 StaticTable 注入本目录与发现目录，消除读侧对写侧的反向依赖。
type Directory struct {
	static    *StaticTable
	mx        sync.Mutex
	instances map[string]port.Registry
}

// NewDirectory 建注册器目录。
func NewDirectory(static *StaticTable) *Directory {
	return &Directory{static: static, instances: make(map[string]port.Registry)}
}

// Get 按地址规格取（或创建）一个注册器。
func (d *Directory) Get(ctx context.Context, spec AddressSpec) (port.Registry, error) {
	key := spec.Type + "/" + spec.Addr

	d.mx.Lock()
	defer d.mx.Unlock()
	if r, ok := d.instances[key]; ok {
		return r, nil
	}

	var r port.Registry
	switch spec.Type {
	case TypeStatic:
		r = d.static
	case TypeRedis:
		rr, err := NewRedisRegistry(ctx, spec.Addr)
		if err != nil {
			return nil, fmt.Errorf("创建注册器失败: %w", err)
		}
		r = rr
	default:
		return nil, fmt.Errorf("注册器不存在: %v", spec.Type)
	}

	d.instances[key] = r
	return r, nil
}

// CloseAll 关闭全部注册器（含静态地址表），可重复调用。
func (d *Directory) CloseAll() {
	d.mx.Lock()
	defer d.mx.Unlock()
	for key, r := range d.instances {
		r.Close()
		delete(d.instances, key)
	}
}
