// Package discover 是模型层 —— 服务发现状态与地址广播的所有者。
//
// 拥有：发现器实例池、Redis 发现器的地址状态机与更新规则。
// 提供：Directory.Get / CloseAll、静态与 Redis 两种实现。
// 允许调用：foundation/port（经 StaticTable 窄接口取静态地址）、
// foundation/resolver、model/endpoint（共享协议，不依赖注册实现）。
// 禁止：import 注册侧的具体实现包、自己定义一套 key 协议、在读取路径上删除注册记录。
package discover

import (
	"context"
	"fmt"
	"sync"

	"github.com/zly-app/component/redis"
	"github.com/zly-app/grpc/foundation/port"
	"github.com/zly-app/grpc/model/endpoint"
)

// Directory 独占发现器实例池。
type Directory struct {
	static       port.StaticTable
	redisCreator redis.Creator
	mx           sync.Mutex
	instances    map[string]port.Discover
}

// NewDirectory 建发现器目录；静态地址表由组合根注入，不再反向依赖注册实现包。
func NewDirectory(static port.StaticTable) *Directory {
	return &Directory{static: static, instances: make(map[string]port.Discover)}
}

// SetRedisCreator 注入 redis 客户端集合（由组合根在 zapp 装配完成后调用）。
func (d *Directory) SetRedisCreator(creator redis.Creator) {
	d.redisCreator = creator
}

// Get 按地址规格取（或创建）一个发现器。
func (d *Directory) Get(ctx context.Context, spec endpoint.Spec) (port.Discover, error) {
	key := spec.Type + "/" + spec.Component

	d.mx.Lock()
	defer d.mx.Unlock()
	if v, ok := d.instances[key]; ok {
		return v, nil
	}

	var v port.Discover
	switch spec.Type {
	case endpoint.TypeStatic:
		v = staticDiscover{table: d.static}
	case endpoint.TypeRedis:
		if d.redisCreator == nil {
			return nil, fmt.Errorf("创建发现器失败: redis creator 未注入（组合根未完成装配）")
		}
		rd, err := NewRedisDiscover(ctx, spec.Component, d.redisCreator)
		if err != nil {
			return nil, fmt.Errorf("创建发现器失败: %w", err)
		}
		v = rd
	default:
		return nil, fmt.Errorf("发现器不存在: %v", spec.Type)
	}

	d.instances[key] = v
	return v, nil
}

// CloseAll 关闭全部发现器；静态表由注册目录统一关闭，这里不重复关。
func (d *Directory) CloseAll() {
	d.mx.Lock()
	defer d.mx.Unlock()
	for key, v := range d.instances {
		if _, isStatic := v.(staticDiscover); !isStatic {
			v.Close()
		}
		delete(d.instances, key)
	}
}
