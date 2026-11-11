package httpgw

import (
	"context"
	"fmt"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/usecase"
)

// Conn 把 HTTP 请求转交给 Forward 用例；path 取不到时按「无 hashKey 规则」处理。
type Conn struct {
	forward *usecase.Forward
	name    string
}

func (c *Conn) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	path, _ := runtime.HTTPPathPattern(ctx)
	return c.forward.Invoke(ctx, c.name, method, path, args, reply, opts...)
}

func (c *Conn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, fmt.Errorf("当前版本不支持stream")
}
