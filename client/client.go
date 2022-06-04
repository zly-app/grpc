package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/opentracing/opentracing-go"
	open_log "github.com/opentracing/opentracing-go/log"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/zly-app/grpc/balance"
	"github.com/zly-app/grpc/registry/static"
)

type Conn = grpc.ClientConn

type GRpcClient struct {
	app core.IApp
}

func NewGRpcConn(app core.IApp, name string, conf *ClientConfig) (*Conn, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("GRpcClient配置检查失败: %v", err)
	}

	// 注册服务地址
	switch strings.ToLower(conf.Registry) {
	case strings.ToLower(static.Name):
		static.RegistryAddress(name, conf.Address)
	default:
		return nil, fmt.Errorf("未定义的GRpc注册器: %v", conf.Registry)
	}

	// 均衡器
	var balanceImpl grpc.DialOption
	switch conf.Balance {
	case strings.ToLower(balance.RoundRobin):
		balanceImpl = balance.NewRoundRobinBalance()
	default:
		return nil, fmt.Errorf("未定义的GRpc均衡器: %v", conf.Balance)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(conf.DialTimeout)*time.Millisecond)
	defer cancel()

	opts := []grpc.DialOption{
		balanceImpl,      // 均衡器
		grpc.WithBlock(), // 等待连接成功. 注意, 这个不要作为配置项, 因为要返回已连接完成的conn, 所以它是必须的.
	}
	if conf.InsecureDial {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials())) // 不安全连接
	}
	var chainUnaryClientList []grpc.UnaryClientInterceptor
	if conf.EnableOpenTrace {
		chainUnaryClientList = append(chainUnaryClientList, UnaryClientOpenTraceInterceptor)
	}
	chainUnaryClientList = append(chainUnaryClientList,
		UnaryClientLogInterceptor(app, conf), // 日志
	)
	opts = append(opts, grpc.WithUnaryInterceptor(grpc_middleware.ChainUnaryClient(chainUnaryClientList...)))

	target := fmt.Sprintf("%s://%s/%s", conf.Registry, "", name)
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc客户端连接失败: %v", err)
	}
	return conn, nil
}

type TextMapCarrier struct {
	metadata.MD
}

func (t TextMapCarrier) Set(key, val string) {
	t.MD[key] = append(t.MD[key], val)
}

// 日志
func UnaryClientLogInterceptor(app core.IApp, conf *ClientConfig) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		log := app.NewTraceLogger(ctx, zap.String("grpc.method", method))

		startTime := time.Now()
		if conf.ReqLogLevelIsInfo {
			log.Info("grpc.request", zap.Any("req", req))
		} else {
			log.Debug("grpc.request", zap.Any("req", req))
		}

		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			log.Error("grpc.response", zap.String("latency", time.Since(startTime).String()), zap.Error(err))
			return err
		}

		if conf.RspLogLevelIsInfo {
			log.Info("grpc.response", zap.String("latency", time.Since(startTime).String()), zap.Any("reply", reply))
		} else {
			log.Debug("grpc.response", zap.String("latency", time.Since(startTime).String()), zap.Any("reply", reply))
		}

		return err
	}
}

// 开放链路追踪hook
func UnaryClientOpenTraceInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	span := utils.Trace.GetChildSpan(ctx, method)
	defer span.Finish()
	ctx = opentracing.ContextWithSpan(ctx, span)

	// 取出元数据
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		// 如果对元数据修改必须使用它的副本
		md = md.Copy()
	} else {
		md = metadata.New(nil)
	}

	// 注入
	carrier := TextMapCarrier{md}
	_ = opentracing.GlobalTracer().Inject(span.Context(), opentracing.TextMap, carrier)
	ctx = metadata.NewOutgoingContext(ctx, md)

	span.SetTag("target", cc.Target())
	span.LogFields(open_log.Object("req", req))
	err := invoker(ctx, method, req, reply, cc, opts...)
	if err != nil {
		span.SetTag("error", true)
		span.LogFields(open_log.Error(err))
	} else {
		span.LogFields(open_log.Object("reply", reply))
	}
	return err
}
