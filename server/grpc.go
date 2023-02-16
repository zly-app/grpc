package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/zly-app/zapp/component/gpool"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/logger"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/zly-app/grpc/gateway"
	"github.com/zly-app/grpc/pkg"
)

type ServiceRegistrar = grpc.ServiceRegistrar
type GrpcServerHandler = func(server ServiceRegistrar)

type GRpcServer struct {
	app    core.IApp
	conf   *ServerConfig
	server *grpc.Server
	gw     *gateway.Gateway
}

func NewGRpcServer(app core.IApp, conf *ServerConfig, hooks ...ServerHook) (*GRpcServer, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("GrpcServer配置检查失败: %v", err)
	}

	gPool := gpool.NewGPool(&gpool.GPoolConfig{
		JobQueueSize: conf.MaxReqWaitQueueSize,
		ThreadCount:  conf.ThreadCount,
	})
	chainUnaryClientList := []grpc.UnaryServerInterceptor{}
	chainUnaryClientList = append(chainUnaryClientList,
		grpc_ctxtags.UnaryServerInterceptor(), // 设置标记
		ReturnErrorInterceptor(app, conf),     // 返回错误拦截
		UnaryServerOpenTraceInterceptor,       // trace
		UnaryServerLogInterceptor(app, conf),  // 日志
		GPoolLimitInterceptor(gPool),          // 协程池限制
		RecoveryInterceptor(),                 // panic恢复
	)
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

	gw := gateway.NewGateway(app, conf.HttpBind)
	g := &GRpcServer{
		app:    app,
		server: server,
		conf:   conf,
		gw:     gw,
	}
	return g, nil
}

func (g *GRpcServer) RegistryServerHandler(hs ...func(server ServiceRegistrar)) {
	for _, h := range hs {
		h(g.server)
	}
}

func (g *GRpcServer) RegistryHttpGatewayHandler(hs ...gateway.GrpcHttpGatewayHandler) {
	if g.conf.HttpBind != "" {
		g.gw.RegistryHttpGatewayHandler(hs...)
	}
}

func (g *GRpcServer) Start() error {
	listener, err := net.Listen("tcp", g.conf.Bind)
	if err != nil {
		return err
	}

	if g.conf.HttpBind != "" {
		serverPort := listener.Addr().(*net.TCPAddr).Port
		go func() {
			err := g.gw.StartGateway(serverPort, g.conf.TLSCertFile, g.conf.TLSDomain)
			if err != nil && err != http.ErrServerClosed {
				g.app.Fatal("grpc网关启动失败", zap.Error(err))
			}
		}()
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
	if g.conf.HttpBind != "" {
		g.gw.Close()
	}
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

// 日志拦截器
func UnaryServerLogInterceptor(app core.IApp, conf *ServerConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		startTime := time.Now()

		reqLogFields := []interface{}{
			ctx,
			"grpc.request",
			zap.String("grpc.method", info.FullMethod),
			zap.Any("req", req),
		}
		if conf.ReqLogLevelIsInfo {
			app.Info(reqLogFields...)
		} else {
			app.Debug(reqLogFields...)
		}

		reply, err := handler(ctx, req)

		if err != nil {
			logFields := []interface{}{
				ctx,
				"grpc.response",
				zap.String("grpc.method", info.FullMethod),
				zap.String("latency", time.Since(startTime).String()),
				zap.Uint32("code", uint32(status.Code(err))),
				zap.Error(err),
			}

			hasPanic := grpc_ctxtags.Extract(ctx).Has(ctxTagHasPanic)
			if hasPanic {
				panicErrDetail := utils.Recover.GetRecoverErrorDetail(err)
				logFields = append(logFields, zap.Bool("panic", true), zap.String("panic.detail", panicErrDetail))
			}

			app.Error(logFields...)
			return reply, err
		}

		replyLogFields := []interface{}{
			ctx,
			"grpc.response",
			zap.String("grpc.method", info.FullMethod),
			zap.String("latency", time.Since(startTime).String()),
			zap.Any("reply", reply),
		}
		if conf.RspLogLevelIsInfo {
			app.Info(replyLogFields...)
		} else {
			app.Debug(replyLogFields...)
		}

		return reply, err
	}
}

func GPoolLimitInterceptor(pool core.IGPool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (reply interface{}, err error) {
		err, ok := pool.TryGoSync(func() error {
			pkg.TraceReq(ctx, req)
			reply, err = handler(ctx, req)
			return err
		})

		if !ok { // 没有执行
			err = errors.New("gPool Limit")
		}
		return reply, err
	}
}

// 开放链路追踪hook
func UnaryServerOpenTraceInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx = pkg.TraceStart(ctx, info.FullMethod)
	defer pkg.TraceEnd(ctx)

	reply, err := handler(ctx, req)
	pkg.TraceReply(ctx, reply, err)
	return reply, err
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

// 获取logger
func GetLogger(ctx context.Context) core.ILogger {
	log := utils.Ctx.GetLogger(ctx)
	if log != nil {
		return log
	}
	return logger.Log
}
