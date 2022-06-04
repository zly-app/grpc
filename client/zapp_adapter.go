package client

import (
	"fmt"

	"github.com/zly-app/zapp/component/conn"
	"github.com/zly-app/zapp/core"
	"google.golang.org/grpc"
)

// 默认组件类型
const DefaultComponentType core.ComponentType = "grpc"

type ClientConnInterface = grpc.ClientConnInterface
type GRpcClientCreator = func(cc ClientConnInterface) interface{}

type IGRpcClientCreator interface {
	// 注册grpc客户端创造者, 这个方法应该在app.Run之前调用
	RegistryGRpcClientCreator(name string, creator GRpcClientCreator)
	// 获取grpc客户端, 如果未注册grpc客户端建造者会panic
	GetGRpcClient(name string) interface{}
	// 关闭客户端
	Close()
}

type instance struct {
	cc     *grpc.ClientConn
	client interface{}
}

func (i *instance) Close() {
	_ = i.cc.Close()
}

type ClientCreatorAdapter struct {
	app           core.IApp
	conn          *conn.Conn
	componentType core.ComponentType

	creatorMap map[string]GRpcClientCreator
}

func NewGRpcClientCreator(app core.IApp, componentType ...core.ComponentType) IGRpcClientCreator {
	c := &ClientCreatorAdapter{
		app:           app,
		conn:          conn.NewConn(),
		componentType: DefaultComponentType,

		creatorMap: make(map[string]GRpcClientCreator),
	}
	if len(componentType) > 0 {
		c.componentType = componentType[0]
	}
	return c
}

func (c *ClientCreatorAdapter) RegistryGRpcClientCreator(name string, creator GRpcClientCreator) {
	c.creatorMap[name] = creator
}

func (c *ClientCreatorAdapter) GetGRpcClient(name string) interface{} {
	return c.conn.GetInstance(c.makeClient, name).(*instance).client
}

func (c *ClientCreatorAdapter) Close() {
	c.conn.CloseAll()
}

func (c *ClientCreatorAdapter) makeClient(name string) (conn.IInstance, error) {
	// 获取建造者
	creator, ok := c.creatorMap[name]
	if !ok {
		return nil, fmt.Errorf("未注册grpc客户端建造者: %v", name)
	}

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
	client := creator(cc)

	return &instance{
		cc:     cc,
		client: client,
	}, nil
}
