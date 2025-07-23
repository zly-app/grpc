package client

import (
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/handler"
	"google.golang.org/grpc"
)

type ClientHook = grpc.UnaryClientInterceptor

var clientHooks = make(map[string][]ClientHook)
var allClientHooks = make([]ClientHook, 0)

// 给指定服务的client添加hook
func RegistryClientHook(serverName string, hooks ...ClientHook) {
	clientHooks[serverName] = append(clientHooks[serverName], hooks...)
}

// 给所有client添加hook
func RegistryAllClientHook(hooks ...ClientHook) {
	allClientHooks = append(allClientHooks, hooks...)
}

func getClientHook(serverName string) grpc.UnaryClientInterceptor {
	hooks := clientHooks[serverName]
	return grpc_middleware.ChainUnaryClient(hooks...)
}

func init() {
	zapp.AddHandler(zapp.BeforeStartHandler, func(app core.IApp, handlerType handler.HandlerType) {
		for serverName, hooks := range clientHooks {
			h := make([]ClientHook, 0, len(allClientHooks)+len(hooks))
			h = append(h, allClientHooks...)
			h = append(h, hooks...)
			clientHooks[serverName] = h
		}
	})
}
