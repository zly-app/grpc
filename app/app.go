// Package app 是应用壳 / 组合根 —— 唯一装配点。
//
// 拥有：全部模型实例与用例实例，以及它们之间 port 的装配关系。
// 提供：New / Options / Server / ClientConn / GatewayConn / Mux / Close。
// 允许调用：全部层（组合根特权）。禁止：在壳里长出业务规则或生命周期时序判定。
//
// 与 zapp 的接壤契约：zapp 的 service 建造者表是进程级全局，因此这里用
// creatorOnce + 原子指针做一次桥接；除此之外本仓库没有任何包级可变业务状态。
package app

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/zly-app/zapp"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/service"
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/adapter/grpcclient"
	"github.com/zly-app/grpc/adapter/grpcserver"
	"github.com/zly-app/grpc/adapter/httpgw"
	"github.com/zly-app/grpc/foundation/addr"
	"github.com/zly-app/grpc/foundation/port"
	"github.com/zly-app/grpc/model/balance"
	"github.com/zly-app/grpc/model/code"
	"github.com/zly-app/grpc/model/config"
	"github.com/zly-app/grpc/model/connreg"
	"github.com/zly-app/grpc/model/discover"
	"github.com/zly-app/grpc/model/hooks"
	"github.com/zly-app/grpc/model/registry"
	"github.com/zly-app/grpc/model/route"
	"github.com/zly-app/grpc/model/serverreg"
	"github.com/zly-app/grpc/usecase"
)

// Option 是装配期的一次选择，不进模型。
type Option func(*options)

type options struct {
	serverHooks        []grpc.UnaryServerInterceptor
	perClientHooks     map[string][]grpc.UnaryClientInterceptor
	allClientHooks     []grpc.UnaryClientInterceptor
	enableServer       bool
	enableGateway      bool
	installErrCodeHook bool
	extractIP          func(*http.Request) string
}

func newOptions(opts ...Option) *options {
	o := &options{
		perClientHooks:     make(map[string][]grpc.UnaryClientInterceptor),
		installErrCodeHook: true,
	}
	for _, f := range opts {
		f(o)
	}
	return o
}

// WithServer 启用 gRPC 服务端（zapp service "grpc"）。
func WithServer() Option { return func(o *options) { o.enableServer = true } }

// WithGateway 启用 HTTP 网关（zapp service "grpc-gateway"）。
func WithGateway() Option { return func(o *options) { o.enableGateway = true } }

// WithServerHooks 给所有 server 追加 unary 拦截器。
func WithServerHooks(hs ...grpc.UnaryServerInterceptor) Option {
	return func(o *options) { o.serverHooks = append(o.serverHooks, hs...) }
}

// WithClientHooks 给指定服务追加客户端拦截器。
func WithClientHooks(name string, hs ...grpc.UnaryClientInterceptor) Option {
	return func(o *options) { o.perClientHooks[name] = append(o.perClientHooks[name], hs...) }
}

// WithAllClientHooks 给所有客户端追加拦截器。
func WithAllClientHooks(hs ...grpc.UnaryClientInterceptor) Option {
	return func(o *options) { o.allClientHooks = append(o.allClientHooks, hs...) }
}

// WithoutErrCodeHook 关闭「把 gRPC 错误码规则装到 zapp filter 链」这一接壤装配。
func WithoutErrCodeHook() Option {
	return func(o *options) { o.installErrCodeHook = false }
}

// WithRequestIP 覆盖网关的 IP 提取策略。
func WithRequestIP(fn func(*http.Request) string) Option {
	return func(o *options) { o.extractIP = fn }
}

// App 是装配体，持有全部实例。
type App struct {
	opt *options

	static    *registry.StaticTable
	regDir    *registry.Directory
	discDir   *discover.Directory
	balancer  *balance.Table
	hookTable *hooks.Table
	decls     *serverreg.Table
	conns     *connreg.Collection
	routes    *route.Table
	mapper    code.Mapper

	serverBuilder *grpcserver.Builder
	clientFactory *grpcclient.Factory
	serve         *usecase.Serve
	dial          *usecase.Dial
	forward       *usecase.Forward
	gateway       *httpgw.Gateway

	zappApp    core.IApp
	attachOnce sync.Once
	closeOnce  sync.Once
}

// creatorOnce / current 是与 zapp 全局 service 建造者表的桥，不是业务状态。
var (
	creatorOnce sync.Once
	current     atomic.Pointer[App]
)

// New 是唯一组合根：构造全部模型、注入 port、显式完成所有装配动作。
// zapp 自身限制「一进程一 app」，重复 New 会覆盖 current，但建造者只注册一次。
func New(opts ...Option) *App {
	o := newOptions(opts...)
	rt := &App{opt: o, mapper: code.Mapper{}}

	// 底座与模型
	rt.static = registry.NewStaticTable()
	rt.regDir = registry.NewDirectory(rt.static)
	rt.discDir = discover.NewDirectory(rt.static)
	rt.balancer = balance.New()
	rt.balancer.Install() // 显式，替代旧的 4 个均衡器 init()
	if o.installErrCodeHook {
		code.Install() // 显式，替代旧的根包 init() 覆写依赖方全局
	}

	rt.hookTable = hooks.New()
	for name, hs := range o.perClientHooks {
		rt.hookTable.AddClient(name, hs...)
	}
	rt.hookTable.AddAllClient(o.allClientHooks...)
	rt.decls = serverreg.New(o.serverHooks...)
	rt.routes = route.New(nil)

	// 用例
	rt.conns = connreg.New(rt.makeConn)
	rt.dial = usecase.NewDial(rt.conns)
	rt.forward = usecase.NewForward(rt.dial, rt.routes)

	// 与 zapp 生命周期接壤：creator 只做委托，装配在 AfterInitialize 完成
	creatorOnce.Do(func() {
		service.RegisterCreatorFunc(config.ServiceTypeGRPC, func(core.IApp) core.IService {
			return &serviceAdapter{rt: current.Load(), role: roleServer}
		})
		service.RegisterCreatorFunc(config.ServiceTypeGateway, func(core.IApp) core.IService {
			return &serviceAdapter{rt: current.Load(), role: roleGateway}
		})
	})
	current.Store(rt)
	return rt
}

// Options 返回需要传给 zapp.NewApp 的选项；用户必须调用它完成装配。
func (a *App) Options() []zapp.Option {
	out := []zapp.Option{
		zapp.WithHandler(zapp.AfterInitializeHandler, func(z core.IApp, _ zapp.HandlerType) {
			a.attach(z)
		}),
		zapp.WithHandler(zapp.BeforeExitHandler, func(z core.IApp, _ zapp.HandlerType) {
			a.Close()
		}),
	}
	if a.opt.enableServer {
		out = append(out, zapp.WithService(config.ServiceTypeGRPC))
	}
	if a.opt.enableGateway {
		out = append(out, zapp.WithService(config.ServiceTypeGateway))
	}
	return out
}

// Server 声明一个具名 gRPC server；同名重复声明显式 panic。
func (a *App) Server(name string, hs ...grpc.UnaryServerInterceptor) grpc.ServiceRegistrar {
	r, err := a.decls.Declare(name, hs...)
	if err != nil {
		panic(err)
	}
	return r
}

// ServerDesc 声明一个以 pb ServiceName 命名的 server。
func (a *App) ServerDesc(hs ...grpc.UnaryServerInterceptor) grpc.ServiceRegistrar {
	r, err := a.decls.Declare("", hs...)
	if err != nil {
		panic(err)
	}
	return r
}

// ClientConn 返回编排好的客户端连接（按需建立）。
func (a *App) ClientConn(name string) grpc.ClientConnInterface {
	return a.dial.Conn(name)
}

// GatewayConn 返回注入网关路由 hashKey 的客户端连接。
func (a *App) GatewayConn(name string) grpc.ClientConnInterface {
	if a.gateway == nil {
		panic("grpc 网关未装配：请把 Options() 传给 zapp.NewApp 并以 WithGateway 启用")
	}
	return a.gateway.Conn(name)
}

// Mux 返回网关 mux，供业务代码注册 pb handler。
func (a *App) Mux() *runtime.ServeMux {
	if a.gateway == nil {
		panic("grpc 网关未装配：请把 Options() 传给 zapp.NewApp 并以 WithGateway 启用")
	}
	return a.gateway.Mux()
}

// Close 统一关闭全部实例，幂等。
func (a *App) Close() {
	a.closeOnce.Do(func() {
		if a.serve != nil {
			a.serve.Close()
		}
		if a.gateway != nil {
			a.gateway.Close()
		}
		a.conns.CloseAll()
		a.discDir.CloseAll()
		a.regDir.CloseAll()
		a.static.Close()
	})
}

func (a *App) attach(z core.IApp) {
	a.attachOnce.Do(func() {
		a.zappApp = z

		gwConf := config.NewGatewayConfig()
		if err := z.GetConfig().ParseServiceConfig(config.ServiceTypeGateway, gwConf, true); err != nil {
			panic(fmt.Errorf("grpc网关服务配置错误: %w", err))
		}
		if err := gwConf.Check(); err != nil {
			panic(fmt.Errorf("grpc网关服务配置检查失败: %w", err))
		}
		a.routes = route.New(gwConf.Route)
		a.forward = usecase.NewForward(a.dial, a.routes)

		a.serverBuilder = grpcserver.NewBuilder(z)
		a.clientFactory = grpcclient.NewFactory(z, a.discDir, a.balancer, a.hookTable)
		a.serve = usecase.NewServe(z, a.decls, a.serverBuilder, a.regDir, func(name string) (*config.ServerConfig, error) {
			return readServerConfig(z, name)
		})

		gw, err := httpgw.New(z, gwConf, a.forward, a.opt.extractIP)
		if err != nil {
			panic(fmt.Errorf("创建grpc网关服务失败: %w", err))
		}
		a.gateway = gw
	})
}

func (a *App) makeConn(name string) (port.Conn, error) {
	z := a.zappApp
	if z == nil {
		return nil, fmt.Errorf("grpc 组合根未装配：请把 Options() 传给 zapp.NewApp")
	}
	conf, err := readClientConfig(z, name)
	if err != nil {
		return nil, err
	}

	// 静态地址由业务层经窄接口注入唯一的地址表，不再直写注册侧单例。
	spec := registry.ParseAddress(conf.Address)
	if spec.Type == registry.TypeStatic && spec.Addr != "" {
		for _, s := range strings.Split(spec.Addr, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			info, err := addr.Parse(s)
			if err != nil {
				return nil, err
			}
			if err := a.static.Register(z.BaseContext(), name, info); err != nil {
				return nil, err
			}
		}
	}
	return a.clientFactory.Create(z.BaseContext(), name, conf)
}

func readClientConfig(z core.IApp, name string) (*config.ClientConfig, error) {
	c := config.NewClientConfig()
	if err := z.GetConfig().ParseComponentConfig(config.ComponentType, name, c, true); err != nil {
		return nil, fmt.Errorf("grpc客户端配置错误: %w", err)
	}
	if err := c.Check(); err != nil {
		return nil, err
	}
	return c, nil
}

func readServerConfig(z core.IApp, name string) (*config.ServerConfig, error) {
	c := config.NewServerConfig()
	key := core.ServiceType(string(config.ServiceTypeGRPC) + "." + name)
	if err := z.GetConfig().ParseServiceConfig(key, c, true); err != nil {
		return nil, err
	}
	if err := c.Check(); err != nil {
		return nil, err
	}
	return c, nil
}
