package gateway

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/handler"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

const (
	GatewayMetadataMethod  = "gw.method"
	GatewayMetadataPath    = "gw.path-bin"
	GatewayMetadataHeaders = "gw.headers-bin"
	GatewayMetadataIP      = "gw.ip-bin"
	GatewayMetadataParams  = "gw.params-bin"
)

type Gateway struct {
	app      core.IApp
	httpBind string // 网关bind
	gwMux    *runtime.ServeMux
	server   *http.Server
}

func NewGateway(app core.IApp, httpBind string) *Gateway {
	gwMux := runtime.NewServeMux(runtime.WithMetadata(gatewayMetadataAnnotator))
	server := &http.Server{
		Addr:    httpBind,
		Handler: gwMux,
	}
	return &Gateway{
		app:      app,
		httpBind: httpBind,
		gwMux:    gwMux,
		server:   server,
	}
}

func (g *Gateway) GetMux() *runtime.ServeMux {
	return g.gwMux
}

func (g *Gateway) StartGateway(closeWaitSec int) error {
	listener, err := net.Listen("tcp", g.httpBind)
	if err != nil {
		return err
	}

	handler.AddHandler(handler.BeforeExitHandler, func(app core.IApp, handlerType handler.HandlerType) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(closeWaitSec)*time.Second)
		defer cancel()

		err := g.server.Shutdown(ctx)
		if err != nil {
			g.app.Error("关闭grpc网关服务失败", zap.Error(err))
		}
	})

	g.app.Info("正在启动grpc网关服务", zap.String("bind", listener.Addr().String()))
	return g.server.Serve(listener)
}

// grpc元数据注解器
func gatewayMetadataAnnotator(ctx context.Context, req *http.Request) metadata.MD {
	method := req.Method
	path := req.URL.Path
	params, _ := url.QueryUnescape(req.URL.RawQuery)
	ip := RequestExtractIP(req)
	headers := req.Header

	headersStorage := make([]string, 0, len(headers)*2)
	for k, vv := range headers {
		headersStorage = append(headersStorage, k, strings.Join(vv, ";"))
	}

	return metadata.MD{
		GatewayMetadataMethod:  {method},
		GatewayMetadataPath:    {path},
		GatewayMetadataParams:  {params},
		GatewayMetadataIP:      {ip},
		GatewayMetadataHeaders: headersStorage,
	}
}
