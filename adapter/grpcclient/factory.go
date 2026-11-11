// Package grpcclient 是接口层 —— gRPC 客户端 transport 适配，不含业务规则。
// 拥有：单条 ClientConn 的拨号选项、TLS 与代理凭证。
// 提供：Factory.Create（实现 usecase.ConnFactory）、Conn（实现 port.Conn）。
// 允许调用：foundation、model/config、model/discover、model/balance、model/registry、model/hooks。
// 禁止：判定「该连谁」（那是均衡/发现模型的规则）；禁止持有其它客户端的连接集合。
package grpcclient

import (
	"context"
	"fmt"
	"net"

	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zly-app/grpc/foundation/port"
	"github.com/zly-app/grpc/model/config"
	"github.com/zly-app/grpc/model/discover"
	"github.com/zly-app/grpc/model/endpoint"
	"github.com/zly-app/grpc/model/hooks"
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

// Create 实现业务层的 ConnFactory port：发现器 → 均衡器 → 一条 ClientConn。
//
// 不再建连接池：gRPC 的 ClientConn 并发安全且按 HTTP/2 流多路复用，
// 每个实例一条 SubConn 已经由均衡器覆盖；再套一层独占借还的池会
// 把并发压到池上限，并把连接数放大成「池大小 × 实例数」。
func (f *Factory) Create(ctx context.Context, name string, conf *config.ClientConfig) (port.Conn, error) {
	src, err := endpoint.ParseSource(conf.Address)
	if err != nil {
		return nil, err
	}
	d, err := f.discover.Get(ctx, src.Spec())
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

	target := fmt.Sprintf("%s:///%s", src.Type, name)
	var proxy utils.ISocks5Proxy
	if conf.ProxyAddress != "" {
		p, err := utils.NewSocks5Proxy(conf.ProxyAddress)
		if err != nil {
			return nil, fmt.Errorf("grpc客户端代理创建失败: %w", err)
		}
		proxy = p
	}

	cc, err := f.makeConn(name, grpc.WithResolvers(builder), balancerOpt, target, proxy, conf)
	if err != nil {
		f.app.Error(ctx, "创建grpc连接失败", zap.String("name", name), zap.String("target", target), zap.Error(err))
		return nil, err
	}
	return &Conn{cc: cc}, nil
}

// makeConn 只组装拨号选项。连接本身是惰性的：真正的 TCP 在第一次 RPC
// （或显式 Connect）时建立，失败由 gRPC 自己的退避重连处理。
func (f *Factory) makeConn(name string, resolverOpt, balancerOpt grpc.DialOption,
	target string, proxy utils.ISocks5Proxy, conf *config.ClientConfig) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{
		resolverOpt,
		balancerOpt,
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

	cc, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc客户端连接失败: %w", err)
	}
	return cc, nil
}
