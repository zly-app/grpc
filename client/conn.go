package client

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
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
	"google.golang.org/grpc/status"

	"github.com/zly-app/grpc/balance"
	"github.com/zly-app/grpc/pkg"
	"github.com/zly-app/grpc/registry"
)

type GRpcClient struct {
	app core.IApp
}

func NewGRpcConn(app core.IApp, name string, conf *ClientConfig) (IGrpcConn, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("GRpcClient配置检查失败: %v", err)
	}

	// 获取注册器
	r, err := registry.GetRegistry(strings.ToLower(conf.Registry), conf.Address)
	if err != nil {
		return nil, fmt.Errorf("获取注册器失败: %v", err)
	}
	reg := grpc.WithResolvers(r)

	// 获取均衡器
	balancer, err := balance.GetBalanceDialOption(strings.ToLower(conf.Balance))
	if err != nil {
		return nil, fmt.Errorf("获取均衡器失败: %v", err)
	}

	// 目标
	target := fmt.Sprintf("%s://%s/%s", conf.Registry, "", name)

	// 代理
	var ss5 pkg.ISocks5Proxy
	if conf.ProxyAddress != "" {
		a, err := pkg.NewSocks5Proxy(conf.ProxyAddress, conf.ProxyUser, conf.ProxyPasswd)
		if err != nil {
			return nil, fmt.Errorf("grpc客户端代理创建失败: %v", err)
		}
		ss5 = a
	}

	var connErr error
	var once sync.Once
	var wg sync.WaitGroup
	wg.Add(conf.ConnPoolSize)
	connList := make([]*grpc.ClientConn, conf.ConnPoolSize)
	for i := 0; i < conf.ConnPoolSize; i++ {
		go func(i int) {
			defer wg.Done()
			conn, err := makeConn(app, reg, balancer, target, ss5, conf)
			if err != nil {
				once.Do(func() {
					connErr = err
				})
				return
			}
			connList[i] = conn
		}(i)
	}
	wg.Wait()

	if connErr != nil {
		for _, conn := range connList {
			if conn != nil {
				_ = conn.Close()
			}
		}
		return nil, connErr
	}

	connPool := NewGrpcConnPool(conf, connList)
	return connPool, nil
}

func makeConn(app core.IApp, registry, balancer grpc.DialOption, target string, ss5 pkg.ISocks5Proxy, conf *ClientConfig) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(conf.DialTimeout)*time.Millisecond)
	defer cancel()

	opts := []grpc.DialOption{
		registry,
		balancer,         // 均衡器
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

	if ss5 != nil {
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return ss5.DialContext(ctx, "tcp", s)
		}))
	}

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
			log.Error("grpc.response", zap.String("latency", time.Since(startTime).String()),
				zap.Uint32("code", uint32(status.Code(err))), zap.Error(err))
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
