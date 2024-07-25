package gateway

import (
	"sync"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/logger"
	"github.com/zly-app/zapp/service"
	"go.uber.org/zap"

	"github.com/zly-app/grpc/client"
)

// 默认服务类型
const DefaultServiceType core.ServiceType = "grpc-gateway"

// 启用grpc网关服务
func WithService() zapp.Option {
	service.RegisterCreatorFunc(DefaultServiceType, func(app core.IApp) core.IService {
		return newServiceAdapter(app)
	})
	return zapp.WithService(DefaultServiceType)
}

var (
	defService     *ServiceAdapter
	defServiceOnce sync.Once
)

type ServiceAdapter struct {
	app    core.IApp
	server *Gateway
	conf   *ServerConfig
}

func (s *ServiceAdapter) Inject(a ...interface{}) {}

func (s *ServiceAdapter) Start() error {
	return s.server.StartGateway()
}

func (s *ServiceAdapter) Close() error { return nil }

func newServiceAdapter(app core.IApp) core.IService {
	defServiceOnce.Do(func() {
		conf := NewServerConfig()
		err := app.GetConfig().ParseServiceConfig(DefaultServiceType, conf, true)
		if err != nil {
			logger.Log.Panic("grpc网关服务配置错误", zap.String("serviceType", string(DefaultServiceType)), zap.Error(err))
		}

		g, err := NewGateway(app, conf)
		if err != nil {
			logger.Log.Panic("创建grpc网关服务失败", zap.String("serviceType", string(DefaultServiceType)), zap.Error(err))
		}
		defService = &ServiceAdapter{
			app:    app,
			server: g,
			conf:   conf,
		}
	})
	return defService
}

// 获取网关mux
func GetGatewayMux() *runtime.ServeMux {
	if defService == nil {
		logger.Log.Fatal("grpc 网关服务未启用")
	}
	return defService.server.GetMux()
}

// 获取grpc客户端conn
func GetGatewayClientConn(serverName string) client.ClientConnInterface {
	return newConn(serverName)
}
