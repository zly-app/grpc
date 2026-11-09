package discover

import (
	"context"

	gresolver "google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/foundation/port"
)

// staticDiscover 只经窄接口取静态地址表，不 import 注册侧实现包。
type staticDiscover struct {
	table port.StaticTable
}

func (s staticDiscover) Builder(ctx context.Context, serverName string) (gresolver.Builder, error) {
	return s.table.Builder(ctx, serverName)
}

func (s staticDiscover) Close() {}
