package server

import (
	"fmt"
	"sync"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/logger"
	"github.com/zly-app/zapp/service"
	"go.uber.org/zap"
)

// 默认服务类型
const DefaultServiceType core.ServiceType = "grpc"

// 启用grpc服务
func WithService(hooks ...ServerHook) zapp.Option {
	service.RegisterCreatorFunc(DefaultServiceType, func(app core.IApp) core.IService {
		return newServiceAdapter(app, hooks...)
	})
	return zapp.WithService(DefaultServiceType)
}

// 注册grpc服务handler
func RegistryServerHandler(h GrpcServerHandler) {
	zapp.App().InjectService(DefaultServiceType, h)
}

var (
	defService     *ServiceAdapter
	defServiceOnce sync.Once
)

type ServiceAdapter struct {
	app    core.IApp
	server *GRpcServer
}

func (s *ServiceAdapter) Inject(a ...interface{}) {
	for _, v := range a {
		switch h := v.(type) {
		case GrpcServerHandler:
			s.server.RegistryServerHandler(h)
		default:
			s.app.Fatal("grpc服务注入类型错误", zap.String("Type", fmt.Sprintf("%T", v)))
		}
	}
}

func (s *ServiceAdapter) Start() error {
	return s.server.Start()
}

func (s *ServiceAdapter) Close() error {
	s.server.Close()
	return nil
}

func newServiceAdapter(app core.IApp, hooks ...ServerHook) core.IService {
	defServiceOnce.Do(func() {
		conf := NewServerConfig()
		err := app.GetConfig().ParseServiceConfig(DefaultServiceType, conf, true)
		if err != nil {
			logger.Log.Panic("grpc服务配置错误", zap.String("serviceType", string(DefaultServiceType)), zap.Error(err))
		}

		g, err := NewGRpcServer(app, conf, hooks...)
		if err != nil {
			logger.Log.Panic("创建grpc服务失败", zap.String("serviceType", string(DefaultServiceType)), zap.Error(err))
		}
		defService = &ServiceAdapter{
			app:    app,
			server: g,
		}
	})
	return defService
}

// 获取网关mux
func GetGatewayMux() *runtime.ServeMux {
	if defService == nil || defService.server.gw == nil {
		logger.Log.Fatal("grpc 网关服务未启用")
	}
	return defService.server.gw.GetMux()
}
