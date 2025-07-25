package grpc

import (
	"context"

	"google.golang.org/grpc"

	"github.com/zly-app/grpc/client"
	"github.com/zly-app/grpc/pkg"
)

type ClientConnInterface = client.ClientConnInterface

// 创建grpc客户端建造者
var GetClientConn = client.GetClientConn

type ClientConn = grpc.ClientConn

type UnaryInvoker = grpc.UnaryInvoker
type CallOption = grpc.CallOption
type ClientHook = func(ctx context.Context, method string, req, reply interface{}, cc *ClientConn, invoker UnaryInvoker, opts ...CallOption) error

// 指定目标
var WithTarget = pkg.WithTarget

// 指定key
var WithHashKey = pkg.WithHashKey

// 给指定服务的client添加hook. 必须在 app.Run 之前
var RegistryClientHook = client.RegistryClientHook

// 给所有client添加hook. 必须在 app.Run 之前
var RegistryAllClientHook = client.RegistryAllClientHook
