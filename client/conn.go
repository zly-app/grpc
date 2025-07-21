package client

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/filter"
	"github.com/zly-app/zapp/pkg/utils"
	"github.com/zlyuancn/connpool"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zly-app/grpc/balance"
	"github.com/zly-app/grpc/discover"
	_ "github.com/zly-app/grpc/discover/redis"
	_ "github.com/zly-app/grpc/discover/static"
	"github.com/zly-app/grpc/pkg"
	"github.com/zly-app/grpc/registry/static"
)

type IGrpcConn interface {
	grpc.ClientConnInterface
	Close() error
}

type GRpcClient struct {
	app        core.IApp
	pool       connpool.IConnectPool
	clientName string
}

type filterReq struct {
	Req         interface{}
	GatewayData string
}
type filterRsp struct {
	Rsp interface{}
}

func (g *GRpcClient) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	ctx, chain := filter.GetClientFilter(ctx, string(DefaultComponentType), g.clientName, method)
	meta := filter.GetCallMeta(ctx)
	meta.AddCallersSkip(1)

	ctx, _ = pkg.TraceInjectIn(ctx)
	gd, _ := pkg.GetGatewayDataByOutgoingRaw(ctx)
	r := &filterReq{Req: args, GatewayData: gd}
	sp := &filterRsp{Rsp: reply}
	err := chain.HandleInject(ctx, r, sp, func(ctx context.Context, req, rsp interface{}) error {
		ctx, mdOutCopy := pkg.TraceInjectOut(ctx)

		// 将主调信息传递到下游服务
		meta := filter.GetCallMeta(ctx)
		ctx = pkg.InjectCallerMetaToMD(ctx, mdOutCopy, filter.CallerMeta{
			CallerService: meta.CallerService(),
			CallerMethod:  meta.CallerMethod(),
		})

		r := req.(*filterReq)
		sp := rsp.(*filterRsp)

		ctx, opts = pkg.InjectTargetFromOpts(ctx, opts)  // 注入 target
		ctx, opts = pkg.InjectHashKeyFromOpts(ctx, opts) // 注入 hash key

		conn, err := g.pool.Get(ctx)
		if err != nil {
			return err
		}
		defer g.pool.Put(conn)

		v := conn.GetConn().(*grpc.ClientConn)
		err = v.Invoke(ctx, method, r.Req, sp.Rsp, opts...)

		pkg.TraceInjectGrpcHeader(ctx, opts...)

		return err
	})
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

func (g *GRpcClient) parseAddress(address string) (string, string) {
	k := strings.Index(address, "://")
	if k == -1 {
		return static.Type, address
	}
	return address[:k], address[k+3:]
}

func NewGRpcConn(app core.IApp, name string, conf *ClientConfig) (IGrpcConn, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("GRpcClient配置检查失败: %v", err)
	}

	g := &GRpcClient{
		app:        app,
		clientName: name,
	}
	dType, dAddr := g.parseAddress(conf.Address)
	var creator connpool.Creator = func(ctx context.Context) (interface{}, error) {
		// 获取发现器
		r, err := discover.GetDiscover(app, strings.ToLower(dType), dAddr)
		if err != nil {
			return nil, fmt.Errorf("获取发现器失败: %v", err)
		}
		// 静态发现器特殊逻辑
		if strings.ToLower(dType) == static.Type {
			ss := strings.Split(dAddr, ",")
			for _, s := range ss {
				addrInfo, err := pkg.ParseAddr(s)
				if err != nil {
					return nil, fmt.Errorf("解析addr失败: %v", err)
				}
				err = static.DefStatic.Registry(app.BaseContext(), name, addrInfo)
				if err != nil {
					return nil, err
				}
			}
		}
		// 发现器
		builder, err := r.GetBuilder(app.BaseContext(), name)
		if err != nil {
			return nil, err
		}
		reg := grpc.WithResolvers(builder)

		// 获取均衡器
		balancer, err := balance.GetBalanceDialOption(strings.ToLower(conf.Balance))
		if err != nil {
			return nil, fmt.Errorf("获取均衡器失败: %v", err)
		}

		// 目标
		target := fmt.Sprintf("%s://%s/%s", dType, "", name)

		// 代理
		var ss5 utils.ISocks5Proxy
		if conf.ProxyAddress != "" {
			a, err := utils.NewSocks5Proxy(conf.ProxyAddress)
			if err != nil {
				return nil, fmt.Errorf("grpc客户端代理创建失败: %v", err)
			}
			ss5 = a
		}

		v, err := makeConn(ctx, app, name, reg, balancer, target, ss5, conf)
		if err != nil {
			app.Warn(ctx, "创建conn失败", zap.String("target", target), zap.Error(err))
		}
		return v, err
	}
	var logCreator connpool.Creator = func(ctx context.Context) (interface{}, error) {
		v, err := creator(ctx)
		if err != nil {
			app.Error(ctx, "grpc creator conn err", zap.String("name", name), zap.Error(err))
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
	pool, err := makePool(conf, logCreator, connClose, valid)
	if err != nil {
		return nil, fmt.Errorf("GRpcClient连接池创建失败: %v", err)
	}
	g.pool = pool
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

func makeConn(ctx context.Context, app core.IApp, name string, registry, balancer grpc.DialOption, target string,
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

	if ss5 != nil {
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return ss5.DialContext(ctx, "tcp", s)
		}))
	}

	opts = append(opts,
		grpc.WithChainUnaryInterceptor(getClientHook(name)), // 请求拦截
	)
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc客户端连接失败: %v", err)
	}
	return conn, nil
}
