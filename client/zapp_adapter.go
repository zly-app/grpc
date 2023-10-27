package client

import (
	"fmt"

	"github.com/zly-app/zapp/component/conn"
	"github.com/zly-app/zapp/core"
	"google.golang.org/grpc"
)

// 默认组件类型
const DefaultComponentType core.ComponentType = "grpc"

// 当前组件类型
var nowComponentType = DefaultComponentType

// 设置组件类型, 这个函数应该在 zapp.NewApp 之前调用
func SetComponentType(t core.ComponentType) {
	nowComponentType = t
}

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
	app           core.IApp
	conn          *conn.Conn
	componentType core.ComponentType
}

// 创建grpc客户端建造者
func NewGRpcClientCreator(app core.IApp) IGRpcClientCreator {
	c := &ClientCreatorAdapter{
		app:           app,
		conn:          conn.NewConn(),
		componentType: nowComponentType,
	}
	return c
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
	err := c.app.GetConfig().ParseComponentConfig(c.componentType, name, conf, true)
	if err != nil {
		return nil, fmt.Errorf("grpc客户端的配置错误: %v", err)
	}

	cc, err := NewGRpcConn(c.app, name, conf)
	if err != nil {
		return nil, fmt.Errorf("grpc客户端创建conn失败: %v", err)
	}

	return &instance{cc: cc}, nil
}
