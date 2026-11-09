// Package grpcserver 是接口层 —— gRPC 服务端协议适配，不含业务规则。
//
// 拥有：grpc.Server、监听器、TLS 凭证等 transport 资源。
// 提供：Builder.Build（实现 usecase.ServerBuilder）、Server（实现 port.ServerRunner）。
// 允许调用：foundation、model/config、model/code、zapp core/filter（横切契约）。
// 禁止：判定「该注册谁」「该不该隐藏错误」以外的业务规则；禁止持有注册状态。
package grpcserver

import (
	"fmt"
	"net"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/zly-app/zapp/core"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/zly-app/grpc/foundation/addr"
	"github.com/zly-app/grpc/foundation/port"
	"github.com/zly-app/grpc/model/code"
	"github.com/zly-app/grpc/model/config"
)

// Builder 把服务声明装配成 transport，实现业务层的 ServerBuilder port。
type Builder struct {
	app    core.IApp
	mapper code.Mapper
}

// NewBuilder 建一个服务端装配器。
func NewBuilder(app core.IApp) *Builder {
	return &Builder{app: app, mapper: code.Mapper{}}
}

// Build 构造拦截器链与 grpc.Server；配置规则由 model/config 负责。
func (b *Builder) Build(name string, conf *config.ServerConfig, hs []grpc.UnaryServerInterceptor) (port.ServerRunner, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("grpc服务配置检查失败: %w", err)
	}

	s := &Server{app: b.app, name: name, conf: conf, mapper: b.mapper}
	chain := []grpc.UnaryServerInterceptor{s.returnErrorInterceptor(), s.appFilter}
	if conf.ReqDataValidate {
		if conf.ReqDataValidateAllField {
			chain = append(chain, ValidateAllInterceptor)
		} else {
			chain = append(chain, ValidateInterceptor)
		}
	}

	cred := grpc.Creds(insecure.NewCredentials())
	if conf.TLSCertFile != "" && conf.TLSKeyFile != "" {
		tc, err := credentials.NewServerTLSFromFile(conf.TLSCertFile, conf.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("加载tls文件失败: %w", err)
		}
		cred = grpc.Creds(tc)
	}

	s.server = grpc.NewServer(
		cred,
		grpc.KeepaliveParams(keepalive.ServerParameters{Time: time.Duration(conf.HeartbeatTime) * time.Second}),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(chain...)),
		grpc.ChainUnaryInterceptor(grpc_middleware.ChainUnaryServer(hs...)),
	)
	return s, nil
}

// Server 独占一个 gRPC transport。
type Server struct {
	app      core.IApp
	name     string
	conf     *config.ServerConfig
	mapper   code.Mapper
	server   *grpc.Server
	listener net.Listener
}

// RegisterService 实现 port.ServerRunner。
func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	s.server.RegisterService(desc, impl)
}

// Start 监听并起 serve 协程；端口在返回时已确定，供注册用例构造公告地址。
func (s *Server) Start() error {
	ls, err := net.Listen("tcp", s.conf.Bind)
	if err != nil {
		s.app.Error("grpc监听端口失败", zap.String("serverName", s.name), zap.Error(err))
		return err
	}
	s.listener = ls
	s.app.Info("正在启动grpc服务", zap.String("serverName", s.name), zap.String("bind", ls.Addr().String()))

	go func() {
		if err := s.server.Serve(ls); err != nil {
			s.app.Error("grpc服务退出", zap.String("serverName", s.name), zap.Error(err))
		}
	}()
	return nil
}

// BoundPort 返回真实监听端口，只在 Start 之后有效。
func (s *Server) BoundPort() int {
	return addr.BindPort(s.listener, s.conf.Bind)
}

// Close 优雅停止 transport。
func (s *Server) Close() {
	if s.server == nil {
		return
	}
	s.server.GracefulStop()
	s.app.Warn("grpc服务已关闭", zap.String("serverName", s.name))
}

var _ port.ServerRunner = (*Server)(nil)
