// Package httpgw 是接口层 —— HTTP 网关协议适配，不含业务规则。
//
// 拥有：ServeMux、http.Server、CORS 与响应包装格式。
// 提供：New / Mux / StartGateway / Close / Conn。
// 允许调用：foundation/ctxkey、foundation/wire、model/config、usecase.Forward、zapp core/filter。
// 禁止：判定 hashKey 规则（归 model/hashkey）、持有客户端连接集合、直读组合根私有配置。
package httpgw

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/zly-app/zapp/core"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/zly-app/grpc/foundation/ctxkey"
	"github.com/zly-app/grpc/model/config"
	"github.com/zly-app/grpc/usecase"
)

// Gateway 独占一个 HTTP transport。
type Gateway struct {
	app         core.IApp
	conf        *config.GatewayConfig
	forward     *usecase.Forward
	extractIP   func(*http.Request) string
	mux         *runtime.ServeMux
	httpHandler http.Handler

	mx     sync.Mutex
	server *http.Server
}

// New 构造 mux、marshaler 与请求过滤器。
func New(app core.IApp, conf *config.GatewayConfig, forward *usecase.Forward,
	extractIP func(*http.Request) string) (*Gateway, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("grpc网关配置检查失败: %w", err)
	}
	if extractIP == nil {
		extractIP = DefaultExtractIP
	}

	var marshaler runtime.Marshaler = &runtime.HTTPBodyMarshaler{
		Marshaler: &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		},
	}
	mux := runtime.NewServeMux(
		runtime.WithMetadata(gatewayMetadataAnnotator),
		runtime.WithMarshalerOption(runtime.MIMEWildcard, marshaler),
		runtime.WithForwardResponseRewriter(ForwardResponseRewriter),
	)

	g := &Gateway{app: app, conf: conf, forward: forward, extractIP: extractIP, mux: mux}
	var handler http.Handler = mux
	if conf.CorsAllowAll {
		handler = allowCORS(handler)
	}
	g.httpHandler = g.reqFilter(handler)
	return g, nil
}

// Mux 返回网关 mux，供业务代码注册 pb handler。
func (g *Gateway) Mux() *runtime.ServeMux { return g.mux }

// Conn 返回一个按 hashKey 规则注入 key 的客户端连接。
func (g *Gateway) Conn(name string) grpc.ClientConnInterface {
	return &Conn{forward: g.forward, name: name}
}

// StartGateway 监听并服务 HTTP；关闭由 Close 负责，不挂全局生命周期钩子。
func (g *Gateway) StartGateway() error {
	ls, err := net.Listen("tcp", g.conf.Bind)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: g.httpHandler}
	g.mx.Lock()
	g.server = server
	g.mx.Unlock()

	g.app.Info("正在启动grpc网关服务", zap.String("bind", ls.Addr().String()))
	return server.Serve(ls)
}

// Close 按配置的等待时间优雅关闭。
func (g *Gateway) Close() {
	g.mx.Lock()
	server := g.server
	g.mx.Unlock()
	if server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.conf.CloseWait)*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		g.app.Error("关闭grpc网关服务失败", zap.Error(err))
	}
}

func allowCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "HEAD, GET, POST, PUT, PATCH, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func gatewayMetadataAnnotator(ctx context.Context, _ *http.Request) metadata.MD {
	d := ctxkey.GatewayDataOf(ctx)
	if d == nil {
		return nil
	}
	text, err := sonicMarshal(d)
	if err != nil {
		return nil
	}
	return metadata.MD{ctxkey.GatewayMDataKey: []string{text}}
}
