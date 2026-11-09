// Package grpcclient 是接口层 —— gRPC 客户端 transport 适配，不含业务规则。
//
// 拥有：连接池、DialOption、TLS 与代理凭证。
// 提供：Factory.Create（实现 usecase.ConnFactory）、Conn（实现 port.Conn）。
// 允许调用：foundation、model/config、model/discover、model/balance、model/registry、model/hooks。
// 禁止：判定「该连谁」（那是均衡/发现模型的规则）；禁止持有其它客户端的连接集合。
package grpcclient

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/pkg/utils"
	"github.com/zlyuancn/connpool"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zly-app/grpc/foundation/port"
	"github.com/zly-app/grpc/model/config"
	"github.com/zly-app/grpc/model/discover"
	"github.com/zly-app/grpc/model/hooks"
	"github.com/zly-app/grpc/model/registry"
)

// Factory 装配一条客户端连接所需的全部 transport 依赖。
type Factory struct {
	app      core.IApp
	discover *discover.Directory
	balancer port.Balancer
	hooks    *hooks.Table
}

// NewFactory 所有依赖由组合根注入。
func NewFactory(app core.IApp, d *discover.Directory, b port.Balancer, h *hooks.Table) *Factory {
	return &Factory{app: app, discover: d, balancer: b, hooks: h}
}

// Create 实现业务层的 ConnFactory port：发现器 → 均衡器 → 连接池。
func (f *Factory) Create(ctx context.Context, name string, conf *config.ClientConfig) (port.Conn, error) {
	spec := registry.ParseAddress(conf.Address)
	d, err := f.discover.Get(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("获取发现器失败: %w", err)
	}
	builder, err := d.Builder(ctx, name)
	if err != nil {
		return nil, err
	}
	balancerOpt, err := f.balancer.DialOption(conf.Balance)
	if err != nil {
		return nil, fmt.Errorf("获取均衡器失败: %w", err)
	}

	target := fmt.Sprintf("%s:///%s", spec.Type, name)
	var proxy utils.ISocks5Proxy
	if conf.ProxyAddress != "" {
		p, err := utils.NewSocks5Proxy(conf.ProxyAddress)
		if err != nil {
			return nil, fmt.Errorf("grpc客户端代理创建失败: %w", err)
		}
		proxy = p
	}

	creator := func(c context.Context) (interface{}, error) {
		v, err := f.makeConn(c, name, grpc.WithResolvers(builder), balancerOpt, target, proxy, conf)
		if err != nil {
			f.app.Error(c, "创建grpc连接失败", zap.String("name", name), zap.String("target", target), zap.Error(err))
		}
		return v, err
	}
	pool, err := makePool(conf, creator)
	if err != nil {
		return nil, fmt.Errorf("grpc客户端连接池创建失败: %w", err)
	}
	return &Conn{pool: pool}, nil
}

func makePool(conf *config.ClientConfig, creator connpool.Creator) (connpool.IConnectPool, error) {
	c := &connpool.Config{
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
		ConnClose: func(conn *connpool.Conn) {
			if cc, ok := conn.GetConn().(*grpc.ClientConn); ok {
				_ = cc.Close()
			}
		},
		ValidConnected: func(conn *connpool.Conn) bool {
			cc, ok := conn.GetConn().(*grpc.ClientConn)
			return ok && cc.GetState() == connectivity.Ready
		},
	}
	return connpool.NewConnectPool(c)
}

func (f *Factory) makeConn(ctx context.Context, name string, resolverOpt, balancerOpt grpc.DialOption,
	target string, proxy utils.ISocks5Proxy, conf *config.ClientConfig) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{
		resolverOpt,
		balancerOpt,
		grpc.WithBlock(), // 必须等到连上才返回，连接池才能校验有效性
	}
	if conf.TLSCertFile != "" {
		tc, err := credentials.NewClientTLSFromFile(conf.TLSCertFile, conf.TLSDomain)
		if err != nil {
			return nil, fmt.Errorf("加载tls文件失败: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(tc))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if proxy != nil {
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return proxy.DialContext(ctx, "tcp", s)
		}))
	}
	opts = append(opts, grpc.WithChainUnaryInterceptor(f.hooks.ClientFor(name)))

	cc, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc客户端连接失败: %w", err)
	}
	return cc, nil
}
