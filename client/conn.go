package client

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/pkg/utils"
	"github.com/zlyuancn/connpool"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/zly-app/grpc/balance"
	"github.com/zly-app/grpc/pkg"
	"github.com/zly-app/grpc/registry"
)

type IGrpcConn interface {
	grpc.ClientConnInterface
	Close() error
}

type GRpcClient struct {
	app  core.IApp
	pool connpool.IConnectPool
}

func (g *GRpcClient) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	ctx = pkg.TraceStart(ctx, method)
	defer pkg.TraceEnd(ctx)

	conn, err := g.pool.Get(ctx)
	if err == nil {
		pkg.TraceReq(ctx, args)
		ctx, opts = pkg.InjectTargetToCtx(ctx, opts)
		ctx, opts = pkg.InjectHashKeyToCtx(ctx, opts)
		v := conn.GetConn().(*grpc.ClientConn)
		err = v.Invoke(ctx, method, args, reply, opts...)
		g.pool.Put(conn)
	}

	pkg.TraceReply(ctx, reply, err)
	return err
}

func (g *GRpcClient) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, fmt.Errorf("当前版本不支持stream")
}

func (g *GRpcClient) Close() error {
	g.pool.Close()
	return nil
}
func (g *GRpcClient) getConn(ctx context.Context) (*grpc.ClientConn, error) {
	conn, err := g.pool.Get(ctx)
	if err != nil {
		return nil, err
	}
	v := conn.GetConn().(*grpc.ClientConn)
	return v, nil
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
	var ss5 utils.ISocks5Proxy
	if conf.ProxyAddress != "" {
		a, err := utils.NewSocks5Proxy(conf.ProxyAddress)
		if err != nil {
			return nil, fmt.Errorf("grpc客户端代理创建失败: %v", err)
		}
		ss5 = a
	}

	var creator connpool.Creator = func(ctx context.Context) (interface{}, error) {
		v, err := makeConn(ctx, app, reg, balancer, target, ss5, conf)
		if err != nil {
			app.Warn("创建conn失败", zap.String("target", target), zap.Error(err))
		}
		return v, err
	}
	var connClose connpool.ConnClose = func(conn *connpool.Conn) {
		v, ok := conn.GetConn().(*grpc.ClientConn)
		if ok {
			_ = v.Close()
		}
	}
	var valid connpool.ValidConnected = func(conn *connpool.Conn) bool {
		v, ok := conn.GetConn().(*grpc.ClientConn)
		return ok && v.GetState() == connectivity.Ready
	}
	pool, err := makePool(conf, creator, connClose, valid)
	if err != nil {
		return nil, fmt.Errorf("GRpcClient连接池创建失败: %v", err)
	}

	g := &GRpcClient{
		app:  app,
		pool: pool,
	}
	return g, nil
}

func makePool(conf *ClientConfig, creator connpool.Creator, connClose connpool.ConnClose,
	valid connpool.ValidConnected) (connpool.IConnectPool, error) {
	poolConf := &connpool.Config{
		WaitFirstConn:     conf.WaitFirstConn,
		MinIdle:           conf.MinIdle,
		MaxIdle:           conf.MaxIdle,
		MaxActive:         conf.MaxActive,
		BatchIncrement:    conf.BatchIncrement,
		BatchShrink:       conf.BatchShrink,
		IdleTimeout:       time.Duration(conf.IdleTimeout) * time.Second,
		WaitTimeout:       time.Duration(conf.WaitTimeout) * time.Second,
		MaxWaitConnCount:  conf.MaxWaitConnCount,
		ConnectTimeout:    time.Duration(conf.ConnectTimeout) * time.Second,
		MaxConnLifetime:   time.Duration(conf.MaxConnLifetime) * time.Second,
		CheckIdleInterval: time.Duration(conf.CheckIdleInterval) * time.Second,
		Creator:           creator,
		ConnClose:         connClose,
		ValidConnected:    valid,
	}
	return connpool.NewConnectPool(poolConf)
}

func makeConn(ctx context.Context, app core.IApp, registry, balancer grpc.DialOption, target string,
	ss5 utils.ISocks5Proxy, conf *ClientConfig) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{
		registry,
		balancer,         // 均衡器
		grpc.WithBlock(), // 等待连接成功. 注意, 这个不要作为配置项, 因为要返回已连接完成的conn, 所以它是必须的.
	}

	if conf.TLSCertFile != "" {
		tc, err := credentials.NewClientTLSFromFile(conf.TLSCertFile, conf.TLSDomain)
		if err != nil {
			return nil, fmt.Errorf("加载tls文件失败: %v", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(tc))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials())) // 不安全连接
	}

	chainUnaryClientList := []grpc.UnaryClientInterceptor{}
	chainUnaryClientList = append(chainUnaryClientList,
		ReqTimeoutInterceptor(app, conf),     // 请求超时
		ctxTagsInterceptor(),                 // 设置标记
		UnaryClientLogInterceptor(app, conf), // 日志
		RecoveryInterceptor(),                // panic恢复
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

func ReqTimeoutInterceptor(app core.IApp, conf *ClientConfig) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if conf.ReqTimeout < 1 {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		ctx, cancel := context.WithTimeout(ctx, time.Duration(conf.ReqTimeout)*time.Second)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// 日志
func UnaryClientLogInterceptor(app core.IApp, conf *ClientConfig) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		startTime := time.Now()

		reqLogFields := []interface{}{
			ctx,
			"grpc.request",
			zap.String("grpc.method", method),
			zap.Any("req", req),
		}
		if conf.ReqLogLevelIsInfo {
			app.Info(reqLogFields...)
		} else {
			app.Debug(reqLogFields...)
		}

		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			logFields := []interface{}{
				ctx,
				"grpc.response",
				zap.String("grpc.method", method),
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
			return err
		}

		replyLogFields := []interface{}{
			ctx,
			"grpc.response",
			zap.String("grpc.method", method),
			zap.String("latency", time.Since(startTime).String()),
			zap.Any("reply", reply),
		}
		if conf.RspLogLevelIsInfo {
			app.Info(replyLogFields...)
		} else {
			app.Debug(replyLogFields...)
		}

		return err
	}
}
