package grpc

import (
	"context"

	"github.com/zly-app/zapp"
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/server"
)

type ServiceRegistrar = server.ServiceRegistrar

type UnaryServerInfo = grpc.UnaryServerInfo
type UnaryHandler = grpc.UnaryHandler
type ServerHook = func(ctx context.Context, req interface{}, info *UnaryServerInfo, handler UnaryHandler) (resp interface{}, err error)

// 启用grpc服务
func WithService(hooks ...ServerHook) zapp.Option {
	wrapHooks := make([]server.ServerHook, len(hooks))
	for i, h := range hooks {
		wrapHooks[i] = h
	}
	return server.WithService(wrapHooks...)
}

func Server(serverName string, hooks ...ServerHook) ServiceRegistrar {
	h := make([]server.ServerHook, len(hooks))
	for i := range hooks {
		h[i] = hooks[i]
	}
	return server.Server(serverName, h...)
}

func ServerDesc(hooks ...ServerHook) ServiceRegistrar {
	h := make([]server.ServerHook, len(hooks))
	for i := range hooks {
		h[i] = hooks[i]
	}
	return server.ServerDesc(h...)
}
