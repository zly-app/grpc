package static

import (
	"context"

	"github.com/zly-app/zapp/core"
	"google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/discover"
	"github.com/zly-app/grpc/registry/static"
)

const Name = static.Name

func init() {
	discover.AddCreator(Name, NewManual)
}

type StaticRegistry struct {
}

func (s *StaticRegistry) Close() {}

func (s *StaticRegistry) GetBuilder(ctx context.Context, serverName string) (resolver.Builder, error) {
	return static.DefStatic.GetBuilder(ctx, serverName)
}

// 创建Manual
func NewManual(_ core.IApp, _ string) (discover.Discover, error) {
	sr := &StaticRegistry{}
	return sr, nil
}
