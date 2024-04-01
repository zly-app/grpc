package discover

import (
	"context"
	"fmt"

	"github.com/zly-app/zapp/component/conn"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/handler"
	"google.golang.org/grpc/resolver"
)

func init() {
	handler.AddHandler(handler.AfterExitHandler, func(app core.IApp, handlerType handler.HandlerType) {
		discoverConn.CloseAll()
	})
}

type Discover interface {
	GetBuilder(ctx context.Context, serverName string) (resolver.Builder, error) // 获取builder, 在需要获取client时调用
	Close()                                                                      // 关闭注册器
}

type DiscoverCreator func(app core.IApp, address string) (Discover, error)

var discoverCreator = map[string]DiscoverCreator{}

var discoverConn = conn.NewConn()

func GetDiscover(app core.IApp, discoverType, discoverAddress string) (Discover, error) {
	ins := discoverConn.GetInstance(func(discoverType string) (conn.IInstance, error) {
		creator, ok := discoverCreator[discoverType]
		if !ok {
			return nil, fmt.Errorf("发现器不存在: %v", discoverType)
		}
		r, err := creator(app, discoverAddress)
		if err != nil {
			return nil, fmt.Errorf("创建发现器失败: %v", err)
		}
		return r, nil
	}, discoverType)
	r := ins.(Discover)
	return r, nil
}

func AddCreator(discoverName string, d DiscoverCreator) {
	discoverCreator[discoverName] = d
}
