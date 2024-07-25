package static

import (
	"context"

	"github.com/zly-app/zapp/core"
	"google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/discover"
	"github.com/zly-app/grpc/registry/static"
)

const Type = static.Type

func init() {
	discover.AddCreator(Type, NewManual)
}

var defStaticDiscover discover.Discover = StaticDiscover{}

type StaticDiscover struct{}

func (s StaticDiscover) Close() {}

func (s StaticDiscover) GetBuilder(ctx context.Context, serverName string) (resolver.Builder, error) {
	return static.DefStatic.GetBuilder(ctx, serverName)
}

// 创建Manual
func NewManual(_ core.IApp, _ string) (discover.Discover, error) {
	return defStaticDiscover, nil
}
