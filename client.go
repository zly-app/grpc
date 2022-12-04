package grpc

import (
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/client"
	"github.com/zly-app/grpc/pkg"
)

type ClientConnInterface = client.ClientConnInterface

// 创建grpc客户端建造者
var NewGRpcClientCreator = client.NewGRpcClientCreator

type ClientConn = grpc.ClientConn

// 指定目标
var WithTarget = pkg.WithTarget

// 指定key
var WithHashKey = pkg.WithHashKey
