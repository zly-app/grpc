package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/opentracing/opentracing-go"
	open_log "github.com/opentracing/opentracing-go/log"
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

type GRpcServer struct {
	app    core.IApp
	conf   *ServerConfig
	server *grpc.Server
}

func NewGRpcServer(app core.IApp, conf *ServerConfig) (*GRpcServer, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("GrpcServer配置检查失败: %v", err)
	}

	chainUnaryClientList := []grpc.UnaryServerInterceptor{}

	if conf.EnableOpenTrace {
		chainUnaryClientList = append(chainUnaryClientList, UnaryServerOpenTraceInterceptor)
	}
	chainUnaryClientList = append(chainUnaryClientList,
		UnaryServerLogInterceptor(app, conf),   // 日志
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

func (g *GRpcServer) Start() error {
	listener, err := net.Listen("tcp", g.conf.Bind)
	if err != nil {
		return err
	}

	g.app.Info("正在启动grpc服务", zap.String("bind", listener.Addr().String()))
	return g.server.Serve(listener)
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

	span := opentracing.StartSpan(info.FullMethod, opentracing.ChildOf(parentSpan))
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
