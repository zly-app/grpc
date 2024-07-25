package server

import (
	"context"

	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/logger"
	"github.com/zly-app/zapp/service"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// 默认服务类型
const DefaultServiceType core.ServiceType = "grpc"

type GrpcServerHandler = func(ctx context.Context, server ServiceRegistrar)

func init() {
	service.RegisterCreatorFunc(DefaultServiceType, func(app core.IApp) core.IService {
		defService.app = app
		return defService
	})
}

// 启用grpc服务
func WithService(hooks ...ServerHook) zapp.Option {
	defService.hooks = append(defService.hooks, hooks...)
	return zapp.WithService(DefaultServiceType)
}

var defService = &ServiceAdapter{}

type ServiceAdapter struct {
	app   core.IApp
	hooks []ServerHook

	server []*GRpcServer
}

func (s *ServiceAdapter) Inject(a ...interface{}) {
	logger.Fatal("grpc不支持Inject, 请使用 pb.RegisterXXXServiceServer(grpc.Server(serverName), impl)")
}

func (s *ServiceAdapter) RegisterService(serverName string, desc *grpc.ServiceDesc, impl interface{}, hooks ...ServerHook) {
	conf := NewServerConfig()
	err := s.app.GetConfig().ParseServiceConfig(DefaultServiceType+"."+core.ServiceType(serverName), conf, true)
	if err != nil {
		logger.Log.Panic("grpc服务配置错误", zap.String("serverName", serverName), zap.Error(err))
	}

	hook := s.hooks
	if len(hooks) > 0 {
		hook = make([]ServerHook, 0, len(s.hooks)+len(hooks))
		hook = append(hook, s.hooks...)
		hook = append(hook, hooks...)
	}
	g, err := NewGRpcServer(s.app, conf, hook...)
	if err != nil {
		logger.Log.Panic("创建grpc服务失败", zap.String("serverName", serverName), zap.Error(err))
	}
	g.RegisterService(serverName, desc, impl)
	defService.server = append(defService.server, g)
}

func (s *ServiceAdapter) Start() error {
	for _, g := range s.server {
		err := g.Start()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ServiceAdapter) Close() error {
	for _, g := range s.server {
		g.Close()
	}
	return nil
}

type serverNameCli struct {
	serverName string
	hooks      []ServerHook
}

func (s serverNameCli) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	if s.serverName == "" {
		s.serverName = desc.ServiceName
	}
	defService.RegisterService(s.serverName, desc, impl, s.hooks...)
}

func Server(serverName string, hooks ...ServerHook) ServiceRegistrar {
	return &serverNameCli{serverName: serverName, hooks: hooks}
}

func ServerDesc(hooks ...ServerHook) ServiceRegistrar {
	return &serverNameCli{serverName: "", hooks: hooks}
}
