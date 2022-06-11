package grpc

import (
	"github.com/zly-app/grpc/client"
)

type ClientConnInterface = client.ClientConnInterface

// 创建grpc客户端建造者
var NewGRpcClientCreator = client.NewGRpcClientCreator
