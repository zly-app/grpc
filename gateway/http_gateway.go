package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/bytedance/sonic"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/handler"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/zly-app/grpc/pkg"
)

type Gateway struct {
	app          core.IApp
	bind         string // 网关bind
	gwMux        *runtime.ServeMux
	closeWaitSec int
	httpHandler  http.Handler
}

func NewGateway(app core.IApp, conf *ServerConfig) (*Gateway, error) {
	if err := conf.Check(); err != nil {
		return nil, fmt.Errorf("Grpc网关配置检查失败: %v", err)
	}

	var mar runtime.Marshaler = &runtime.HTTPBodyMarshaler{
		Marshaler: &Marshaler{
			Marshaler: &runtime.JSONPb{
				MarshalOptions: protojson.MarshalOptions{
					EmitUnpopulated: true,
					UseProtoNames:   true,
				},
				UnmarshalOptions: protojson.UnmarshalOptions{
					DiscardUnknown: true,
				},
			},
		},
	}
	gwMux := runtime.NewServeMux(
		runtime.WithMetadata(gatewayMetadataAnnotator),
		runtime.WithMarshalerOption(runtime.MIMEWildcard, mar),
	)
	var httpHandler http.Handler = gwMux
	if conf.CorsAllowAll {
		httpHandler = allowCORS(gwMux)
	}
	return &Gateway{
		app:          app,
		bind:         conf.Bind,
		gwMux:        gwMux,
		closeWaitSec: conf.CloseWait,
		httpHandler:  httpHandler,
	}, nil
}

func (g *Gateway) GetMux() *runtime.ServeMux {
	return g.gwMux
}

func allowCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "HEAD, GET, POST, PUT, PATCH, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (g *Gateway) StartGateway() error {
	listener, err := net.Listen("tcp", g.bind)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: g.httpHandler}
	handler.AddHandler(handler.BeforeExitHandler, func(app core.IApp, handlerType handler.HandlerType) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.closeWaitSec)*time.Second)
		defer cancel()

		err := server.Shutdown(ctx)
		if err != nil {
			g.app.Error("关闭grpc网关服务失败", zap.Error(err))
		}
	})

	g.app.Info("正在启动grpc网关服务", zap.String("bind", listener.Addr().String()))
	return server.Serve(listener)
}

// grpc元数据注解器
func gatewayMetadataAnnotator(ctx context.Context, req *http.Request) metadata.MD {
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))

	d := &pkg.GatewayData{
		Method:   req.Method,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
		RawBody:  string(body),
		IP:       RequestExtractIP(req),
		Headers:  req.Header,
	}
	s, _ := sonic.MarshalString(d)

	return metadata.MD{pkg.GatewayMDataKey: []string{s}}
}
