package server

import (
	"context"
	"fmt"
	"net"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/zly-app/zapp/core"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

type ServiceRegistrar = grpc.ServiceRegistrar
type GrpcServerHandler = func(ctx context.Context, server ServiceRegistrar)

type GRpcServer struct {
	app    core.IApp
	conf   *ServerConfig
	server *grpc.Server
}

func NewGRpcServer(app core.IApp, conf *ServerConfig, hooks ...ServerHook) (*GRpcServer, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("GrpcServer配置检查失败: %v", err)
	}

	chainUnaryClientList := []grpc.UnaryServerInterceptor{
		AppFilter,
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

	server := grpc.NewServer(
		cred,
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time: time.Duration(conf.HeartbeatTime) * time.Second, // 心跳
		}),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(chainUnaryClientList...)),
		grpc.ChainUnaryInterceptor(HookInterceptor(hooks...)), // 请求拦截
	)

	g := &GRpcServer{
		app:    app,
		server: server,
		conf:   conf,
	}
	return g, nil
}

func (g *GRpcServer) RegistryServerHandler(hs ...func(ctx context.Context, server ServiceRegistrar)) {
	ctx := context.Background()
	for _, h := range hs {
		h(ctx, g.server)
	}
}

func (g *GRpcServer) Start() error {
	listener, err := net.Listen("tcp", g.conf.Bind)
	if err != nil {
		return err
	}

	g.app.Info("正在启动grpc服务", zap.String("bind", listener.Addr().String()))
	err = g.server.Serve(listener)
	if err != nil {
		return err
	}
	return nil
}

func (g *GRpcServer) Close() {
	g.server.GracefulStop()
	g.app.Warn("grpc服务已关闭")
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
