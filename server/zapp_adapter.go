package server

import (
	"fmt"

	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/logger"
	"github.com/zly-app/zapp/service"
	"go.uber.org/zap"
)

// 默认服务类型
const DefaultServiceType core.ServiceType = "grpc"

// 当前服务类型
var nowServiceType = DefaultServiceType

// 设置服务类型, 这个函数应该在 zapp.NewApp 之前调用
func SetServiceType(t core.ServiceType) {
	nowServiceType = t
}

// 启用grpc服务
func WithService() zapp.Option {
	service.RegisterCreatorFunc(nowServiceType, func(app core.IApp) core.IService {
		return NewServiceAdapter(app)
	})
	return zapp.WithService(nowServiceType)
}

// 注册grpc服务handler
func RegistryServerHandler(h GrpcServerHandler) {
	zapp.App().InjectService(nowServiceType, h)
}

// 注册grpc服务网关handler
func RegistryHttpGatewayHandler(h GrpcHttpGatewayHandler) {
	zapp.App().InjectService(nowServiceType, h)
}

type ServiceAdapter struct {
	app    core.IApp
	server *GRpcServer
}

func (s *ServiceAdapter) Inject(a ...interface{}) {
	for _, v := range a {
		switch h := v.(type) {
		case GrpcServerHandler:
			s.server.RegistryServerHandler(h)
		case GrpcHttpGatewayHandler:
			s.server.RegistryHttpGatewayHandler(h)
		default:
			s.app.Fatal("grpc服务注入类型错误", zap.String("Type", fmt.Sprintf("%T", v)))
		}
	}
}

func (s *ServiceAdapter) Start() error {
	return s.server.Start()
}

func (s *ServiceAdapter) Close() error {
	return nil
}

func NewServiceAdapter(app core.IApp) core.IService {
	conf := NewServerConfig()
	err := app.GetConfig().ParseServiceConfig(nowServiceType, conf, true)
	if err != nil {
		logger.Log.Panic("grpc服务配置错误", zap.String("serviceType", string(nowServiceType)), zap.Error(err))
	}

	g, err := NewGRpcServer(app, conf)
	if err != nil {
		logger.Log.Panic("创建grpc服务失败", zap.String("serviceType", string(nowServiceType)), zap.Error(err))
	}

	// 在app关闭前优雅的关闭服务
	zapp.AddHandler(zapp.BeforeExitHandler, func(app core.IApp, handlerType zapp.HandlerType) {
		g.Close()
	})

	return &ServiceAdapter{
		app:    app,
		server: g,
	}
}
