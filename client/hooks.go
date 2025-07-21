package client

import (
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
)

type ClientHook = grpc.UnaryClientInterceptor

var clientHooks = make(map[string][]ClientHook)

func RegistryClientHook(serverName string, hooks ...ClientHook) {
	clientHooks[serverName] = append(clientHooks[serverName], hooks...)
}

func getClientHook(serverName string) grpc.UnaryClientInterceptor {
	hooks := clientHooks[serverName]
	return grpc_middleware.ChainUnaryClient(hooks...)
}
