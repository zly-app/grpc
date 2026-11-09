// Package usecase 是业务层 —— 用例编排，不拥有任何规则本体。
//
// 拥有：无数据；三个用例（Serve / Dial / Forward）只调度模型与接口层。
// 提供：Serve.Start / Serve.Close、Dial.Conn、Forward.Invoke。
// 允许调用：model（经公开操作）、foundation/port（窄接口）、zapp core（框架契约）。
// 禁止：在这里写「谁能做X」「什么条件下Y」这类判定；禁止直接读写模型内部状态。
package usecase

import (
	"context"
	"fmt"
	"sync"

	"github.com/zly-app/zapp/core"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/zly-app/grpc/foundation/addr"
	"github.com/zly-app/grpc/foundation/port"
	"github.com/zly-app/grpc/model/config"
	"github.com/zly-app/grpc/model/registry"
	"github.com/zly-app/grpc/model/serverreg"
)

// ServerConfigReader 由组合根注入，屏蔽配置读取细节。
type ServerConfigReader func(name string) (*config.ServerConfig, error)

// ServerBuilder 是业务层需要的 port：把声明变成可运行的 transport。
type ServerBuilder interface {
	Build(name string, conf *config.ServerConfig, hooks []grpc.UnaryServerInterceptor) (port.ServerRunner, error)
}

// RegistryDirectory 是业务层需要的 port：按地址规格取注册器。
type RegistryDirectory interface {
	Get(ctx context.Context, spec registry.AddressSpec) (port.Registry, error)
}

type startedServer struct {
	name     string
	runner   port.ServerRunner
	registry port.Registry
}

// Serve 是「启动并注册 gRPC 服务」用例。
type Serve struct {
	app        core.IApp
	decls      *serverreg.Table
	builder    ServerBuilder
	registries RegistryDirectory
	confOf     ServerConfigReader

	mx      sync.Mutex
	started []startedServer
}

// NewServe 装配用例；所有依赖由组合根注入。
func NewServe(app core.IApp, decls *serverreg.Table, builder ServerBuilder,
	registries RegistryDirectory, confOf ServerConfigReader) *Serve {
	return &Serve{app: app, decls: decls, builder: builder, registries: registries, confOf: confOf}
}

// Start 编排：构建 transport → 绑定 pb 服务 → 监听 → 注册到注册中心。
// 注册动作在监听成功之后同点完成，不再挂 AfterStart 全局钩子。
func (s *Serve) Start() error {
	for _, e := range s.decls.Entries() {
		conf, err := s.confOf(e.Name)
		if err != nil {
			return fmt.Errorf("服务 %s 配置错误: %w", e.Name, err)
		}
		runner, err := s.builder.Build(e.Name, conf, e.Hooks)
		if err != nil {
			return fmt.Errorf("服务 %s 构建失败: %w", e.Name, err)
		}
		for _, b := range e.Bindings {
			runner.RegisterService(b.Desc, b.Impl)
		}
		if err := runner.Start(); err != nil {
			runner.Close()
			return err
		}

		spec := registry.ParseAddress(conf.RegistryAddress)
		reg, err := s.registries.Get(s.app.BaseContext(), spec)
		if err != nil {
			runner.Close()
			return err
		}

		info := &addr.Info{
			Name:     conf.PublishName,
			Endpoint: conf.PublishAddress,
			Weight:   conf.PublishWeight,
		}
		if info.Endpoint == "" {
			info.Endpoint = fmt.Sprintf("%s:%d", s.app.GetConfig().Config().Frame.Instance, runner.BoundPort())
		}
		if info.Name == "" {
			info.Name = e.Name
		}
		if err := reg.Register(s.app.BaseContext(), e.Name, info); err != nil {
			s.app.Error("grpc服务注册失败", zap.String("serverName", e.Name), zap.Any("addr", info), zap.Error(err))
			runner.Close()
			return err
		}
		s.app.Info("grpc服务注册", zap.String("serverName", e.Name), zap.Any("addr", info))

		s.mx.Lock()
		s.started = append(s.started, startedServer{name: e.Name, runner: runner, registry: reg})
		s.mx.Unlock()
	}
	return nil
}

// Close 编排注销与关闭；不再挂 BeforeExit 全局钩子。可重复调用。
func (s *Serve) Close() {
	s.mx.Lock()
	list := s.started
	s.started = nil
	s.mx.Unlock()

	for _, v := range list {
		v.registry.Unregister(context.Background(), v.name)
		v.runner.Close()
	}
}
