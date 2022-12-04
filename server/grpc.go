package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/opentracing/opentracing-go"
	open_log "github.com/opentracing/opentracing-go/log"
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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type ServiceRegistrar = grpc.ServiceRegistrar
type RegistryGrpcServerHandler = func(server ServiceRegistrar)

type RegistryGrpcHttpGatewayHandler = func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error

type GRpcServer struct {
	app                 core.IApp
	conf                *ServerConfig
	server              *grpc.Server
	httpGatewayHandlers []RegistryGrpcHttpGatewayHandler
}

func NewGRpcServer(app core.IApp, conf *ServerConfig) (*GRpcServer, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("GrpcServer配置检查失败: %v", err)
	}

	chainUnaryClientList := []grpc.UnaryServerInterceptor{}

	if !conf.DisableOpenTrace {
		chainUnaryClientList = append(chainUnaryClientList, UnaryServerOpenTraceInterceptor)
	}
	gPool := gpool.NewGPool(&gpool.GPoolConfig{
		JobQueueSize: conf.MaxReqWaitQueueSize,
		ThreadCount:  conf.ThreadCount,
	})
	chainUnaryClientList = append(chainUnaryClientList,
		UnaryServerLogInterceptor(app, conf),   // 日志
		GPoolLimitInterceptor(gPool),           // 协程池限制
		grpc_ctxtags.UnaryServerInterceptor(),  // 设置标记
		grpc_recovery.UnaryServerInterceptor(), // panic恢复
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
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(chainUnaryClientList...)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time: time.Duration(conf.HeartbeatTime) * time.Second, // 心跳
		}),
	)

	g := &GRpcServer{
		app:    app,
		server: server,
		conf:   conf,
	}
	return g, nil
}

func (g *GRpcServer) RegistryServerHandler(hs ...RegistryGrpcServerHandler) {
	for _, h := range hs {
		h(g.server)
	}
}

func (g *GRpcServer) RegistryHttpGatewayHandler(hs ...RegistryGrpcHttpGatewayHandler) {
	for _, h := range hs {
		g.httpGatewayHandlers = append(g.httpGatewayHandlers, h)
	}
}

func (g *GRpcServer) Start() error {
	if g.conf.HttpBind != "" {
		return g.StartGateway()
	}
	listener, err := net.Listen("tcp", g.conf.Bind)
	if err != nil {
		return err
	}

	g.app.Info("正在启动grpc服务", zap.String("bind", listener.Addr().String()))
	return g.server.Serve(listener)
}

func (g *GRpcServer) StartGateway() error {
	listener, err := net.Listen("tcp", g.conf.Bind)
	if err != nil {
		return err
	}
	gatewayListener, err := net.Listen("tcp", g.conf.HttpBind)
	if err != nil {
		return err
	}

	g.app.Info("正在启动grpc服务", zap.String("bind", listener.Addr().String()))
	go func() {
		err := g.server.Serve(listener)
		if err != nil {
			g.app.Error("grpc服务启动失败", zap.Error(err))
		}
		g.app.Info("grpc服务启动成功", zap.String("bind", listener.Addr().String()))
	}()

	serverPort := listener.Addr().(*net.TCPAddr).Port
	g.app.Info("网关客户端正在连接")
	conn, err := g.makeGatewayConn(serverPort)
	if err != nil {
		return err
	}

	gwMux := runtime.NewServeMux()
	for _, h := range g.httpGatewayHandlers {
		err = h(context.Background(), gwMux, conn)
		if err != nil {
			return fmt.Errorf("注册网关handler失败: %v", err)
		}
	}
	gwServer := &http.Server{
		Addr:    g.conf.HttpBind,
		Handler: gwMux,
	}

	g.app.Info("正在启动grpc网关服务", zap.String("bind", gatewayListener.Addr().String()))
	return gwServer.Serve(gatewayListener)
}

func (g *GRpcServer) makeGatewayConn(port int) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(g.app.BaseContext(), time.Second)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithBlock(), // 等待连接成功. 注意, 这个不要作为配置项, 因为要返回已连接完成的conn, 所以它是必须的.
	}

	if g.conf.TLSCertFile != "" {
		tc, err := credentials.NewClientTLSFromFile(g.conf.TLSCertFile, g.conf.TLSDomain)
		if err != nil {
			return nil, fmt.Errorf("grpc网关客户端加载tls文件失败: %v", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(tc))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials())) // 不安全连接
	}

	target := fmt.Sprintf("localhost:%v", port)
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc网关客户端连接失败: %v", err)
	}
	return conn, nil
}

func (g *GRpcServer) Close() {
	g.server.GracefulStop()
	g.app.Warn("grpc服务已关闭")
}

// 日志拦截器
func UnaryServerLogInterceptor(app core.IApp, conf *ServerConfig) grpc.UnaryServerInterceptor {
	interceptorUnknownErr := !app.GetConfig().Config().Frame.Debug && !conf.SendDetailedErrorInProduction // 是否拦截未定义的错误
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		log := app.NewTraceLogger(ctx, zap.String("grpc.method", info.FullMethod))
		ctx = utils.Ctx.SaveLogger(ctx, log)

		startTime := time.Now()
		if conf.ReqLogLevelIsInfo {
			log.Info("grpc.request", zap.Any("req", req))
		} else {
			log.Debug("grpc.request", zap.Any("req", req))
		}

		reply, err := handler(ctx, req)

		if err != nil {
			log.Error("grpc.response", zap.String("latency", time.Since(startTime).String()), zap.Error(err))
			if interceptorUnknownErr && status.Code(err) == codes.Unknown { // 拦截未定义错误
				return reply, errors.New("service internal error")
			}
			return reply, err
		}

		if conf.RspLogLevelIsInfo {
			log.Info("grpc.response", zap.String("latency", time.Since(startTime).String()), zap.Any("reply", reply))
		} else {
			log.Debug("grpc.response", zap.String("latency", time.Since(startTime).String()), zap.Any("reply", reply))
		}

		return reply, err
	}
}

func GPoolLimitInterceptor(pool core.IGPool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (reply interface{}, err error) {
		err, ok := pool.TryGoSync(func() error {
			reply, err = handler(ctx, req)
			return err
		})
		if !ok {
			return nil, errors.New("gPool Limit")
		}
		return reply, err
	}
}

type TextMapCarrier struct {
	metadata.MD
}

func (t TextMapCarrier) ForeachKey(handler func(key, val string) error) error {
	for k, v := range t.MD {
		for _, vv := range v {
			if err := handler(k, vv); err != nil {
				return err
			}
		}
	}
	return nil
}

// 开放链路追踪hook
func UnaryServerOpenTraceInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// 取出元数据
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		// 如果对元数据修改必须使用它的副本
		md = md.Copy()
	}

	// 从元数据中取出span
	carrier := TextMapCarrier{md}
	parentSpan, _ := opentracing.GlobalTracer().Extract(opentracing.TextMap, carrier)

	span := opentracing.StartSpan("grpc."+info.FullMethod, opentracing.ChildOf(parentSpan))
	defer span.Finish()
	ctx = opentracing.ContextWithSpan(ctx, span)

	span.LogFields(open_log.Object("req", req))
	reply, err := handler(ctx, req)
	if err != nil {
		span.SetTag("error", true)
		span.LogFields(open_log.Error(err))
	} else {
		span.LogFields(open_log.Object("reply", reply))
	}
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
