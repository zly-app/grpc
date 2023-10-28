package client

import (
	"fmt"
	"sync"

	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/component/conn"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/handler"
	"google.golang.org/grpc"
)

// 默认组件类型
const DefaultComponentType core.ComponentType = "grpc"

type ClientConnInterface = grpc.ClientConnInterface
type GRpcClientCreator = func(cc ClientConnInterface) interface{}

type IGRpcClientCreator interface {
	// 获取grpc客户端conn
	GetClientConn(name string) ClientConnInterface
	// 关闭客户端
	Close()
}

type instance struct {
	cc IGrpcConn
}

func (i *instance) Close() {
	_ = i.cc.Close()
}

type ClientCreatorAdapter struct {
	app  core.IApp
	conn *conn.Conn
}

var defGrpcClientCreator IGRpcClientCreator
var defGrpcClientCreatorOnce sync.Once

// 创建grpc客户端建造者
func initGRpcClientCreator() IGRpcClientCreator {
	defGrpcClientCreatorOnce.Do(func() {
		defGrpcClientCreator = &ClientCreatorAdapter{
			app:  zapp.App(),
			conn: conn.NewConn(),
		}
		zapp.AddHandler(zapp.BeforeExitHandler, func(app core.IApp, handlerType handler.HandlerType) {
			defGrpcClientCreator.Close()
		})
	})
	return defGrpcClientCreator
}

func (c *ClientCreatorAdapter) GetClientConn(name string) ClientConnInterface {
	return c.conn.GetInstance(c.makeClient, name).(*instance).cc
}

func (c *ClientCreatorAdapter) Close() {
	c.conn.CloseAll()
}

func (c *ClientCreatorAdapter) makeClient(name string) (conn.IInstance, error) {
	// 解析配置
	conf := NewClientConfig()
	err := c.app.GetConfig().ParseComponentConfig(DefaultComponentType, name, conf, true)
	if err != nil {
		return nil, fmt.Errorf("grpc客户端的配置错误: %v", err)
	}

	cc, err := NewGRpcConn(c.app, name, conf)
	if err != nil {
		return nil, fmt.Errorf("grpc客户端创建conn失败: %v", err)
	}

	return &instance{cc: cc}, nil
}

// 获取grpc客户端conn
func GetClientConn(name string) ClientConnInterface {
	initGRpcClientCreator()
	return defGrpcClientCreator.GetClientConn(name)
}
