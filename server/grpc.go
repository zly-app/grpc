package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/handler"
	"github.com/zly-app/zapp/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/zly-app/grpc/pkg"
	"github.com/zly-app/grpc/registry"
	_ "github.com/zly-app/grpc/registry/redis"
	_ "github.com/zly-app/grpc/registry/static"
)

type ServiceRegistrar = grpc.ServiceRegistrar

type GRpcServer struct {
	app    core.IApp
	conf   *ServerConfig
	server *grpc.Server

	serverName  string
	serviceDesc *grpc.ServiceDesc
}

func NewGRpcServer(app core.IApp, conf *ServerConfig, hooks ...ServerHook) (*GRpcServer, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("GrpcServer配置检查失败: %v", err)
	}
	g := &GRpcServer{
		app:  app,
		conf: conf,
	}
	chainUnaryClientList := []grpc.UnaryServerInterceptor{
		g.AppFilter,
		ReturnErrorInterceptor(app, conf), // 返回错误拦截
	}
	if conf.ReqDataValidate && !conf.ReqDataValidateAllField {
		chainUnaryClientList = append(chainUnaryClientList, UnaryServerReqDataValidateInterceptor)
	}
	if conf.ReqDataValidate && conf.ReqDataValidateAllField {
		chainUnaryClientList = append(chainUnaryClientList, UnaryServerReqDataValidateAllInterceptor)
	}

	cred := grpc.Creds(insecure.NewCredentials())
	if conf.TLSCertFile != "" && conf.TLSKeyFile != "" {
		tc, err := credentials.NewServerTLSFromFile(conf.TLSCertFile, conf.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("加载tls文件失败: %v", err)
		}
		cred = grpc.Creds(tc)
	}

	g.server = grpc.NewServer(
		cred,
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time: time.Duration(conf.HeartbeatTime) * time.Second, // 心跳
		}),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(chainUnaryClientList...)),
		grpc.ChainUnaryInterceptor(HookInterceptor(hooks...)), // 请求拦截
	)
	return g, nil
}

func (g *GRpcServer) RegisterService(serverName string, desc *grpc.ServiceDesc, impl interface{}) {
	g.server.RegisterService(desc, impl)
	g.serverName = serverName
	g.serviceDesc = desc
}

func (g *GRpcServer) Start() error {
	// 获取注册器
	r, err := registry.GetRegistry(g.app, strings.ToLower(g.conf.RegistryType), g.conf.RegistryName)
	if err != nil {
		logger.Error("grpc 获取注册器失败", zap.String("serverName", g.serverName), zap.Error(err))
		return fmt.Errorf("获取注册器失败: %v", err)
	}

	// 开始监听
	listener, err := net.Listen("tcp", g.conf.Bind)
	if err != nil {
		logger.Error("grpc 监听端口失败", zap.String("serverName", g.serverName), zap.Error(err))
		return err
	}

	isRegistry := int32(1)
	handler.AddHandler(handler.AfterStartHandler, func(app core.IApp, handlerType handler.HandlerType) {
		if atomic.LoadInt32(&isRegistry) != 1 {
			return
		}

		addr := &pkg.AddrInfo{
			Name:     g.conf.PublishName,
			Endpoint: g.conf.PublishAddress,
			Weight:   g.conf.PublishWeight,
		}
		if addr.Endpoint == "" {
			addr.Endpoint = fmt.Sprintf("%s:%d", g.app.GetConfig().Config().Frame.Instance, pkg.ParseBindPort(listener, g.conf.Bind))
		}
		if addr.Name == "" {
			addr.Name = g.serverName
		}

		err = r.Registry(g.app.BaseContext(), g.serverName, addr)
		g.app.Info("grpc服务注册", zap.String("serverName", g.serverName), zap.Any("addr", addr))
		if err != nil {
			g.app.Error("grpc服务注册失败", zap.String("serverName", g.serverName), zap.Any("addr", addr), zap.Error(err))
			g.app.Exit()
		}

		handler.AddHandler(handler.BeforeExitHandler, func(app core.IApp, handlerType handler.HandlerType) {
			r.UnRegistry(context.Background(), g.serverName)
		})
	})

	g.app.Info("正在启动grpc服务", zap.String("serverName", g.serverName), zap.String("bind", listener.Addr().String()))
	go func() {
		err = g.server.Serve(listener)
		atomic.StoreInt32(&isRegistry, 0)
		if err != nil {
			g.app.Fatal("启动grpc服务失败", zap.String("serverName", g.serverName), zap.Error(err))
		}
	}()
	return nil
}

func (g *GRpcServer) Close() {
	g.server.GracefulStop()
	g.app.Warn("grpc服务已关闭", zap.String("serverName", g.serverName))
}

// 错误拦截
func ReturnErrorInterceptor(app core.IApp, conf *ServerConfig) grpc.UnaryServerInterceptor {
	interceptorUnknownErr := !app.GetConfig().Config().Frame.Debug && !conf.SendDetailedErrorInProduction // 是否拦截未定义的错误
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		reply, err := handler(ctx, req)
		if interceptorUnknownErr && err != nil && status.Code(err) == codes.Unknown { // 拦截未定义错误
			return reply, status.Error(codes.Internal, "service internal error")
		}
		return reply, err
	}
}

type ValidateInterface interface {
	Validate() error
}
type ValidateAllInterface interface {
	ValidateAll() error
}

// 数据校验
func UnaryServerReqDataValidateInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if v, ok := req.(ValidateInterface); ok {
		if err := v.Validate(); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	return handler(ctx, req)
}

// 数据校验, 总是校验所有字段
func UnaryServerReqDataValidateAllInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	v, ok := req.(ValidateAllInterface)
	if !ok {
		// 降级
		return UnaryServerReqDataValidateInterceptor(ctx, req, info, handler)
	}
	if err := v.ValidateAll(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return handler(ctx, req)
}
