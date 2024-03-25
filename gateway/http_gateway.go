package gateway

import (
	"context"
	"fmt"
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
)

const (
	GatewayMDataKey = "gw.data-bin"
)

type Gateway struct {
	app          core.IApp
	bind         string // 网关bind
	gwMux        *runtime.ServeMux
	closeWaitSec int
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
	return &Gateway{
		app:          app,
		bind:         conf.Bind,
		gwMux:        gwMux,
		closeWaitSec: conf.CloseWait,
	}, nil
}

func (g *Gateway) GetMux() *runtime.ServeMux {
	return g.gwMux
}

func (g *Gateway) StartGateway() error {
	listener, err := net.Listen("tcp", g.bind)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: g.gwMux}
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
	d := &GatewayData{
		Method:   req.Method,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
		IP:       RequestExtractIP(req),
		Headers:  req.Header,
	}
	s, _ := sonic.MarshalString(d)

	return metadata.MD{GatewayMDataKey: []string{s}}
}

type GatewayData struct {
	Method   string
	Path     string
	RawQuery string
	IP       string
	Headers  http.Header
}

// 获取网关数据
func GetGatewayData(ctx context.Context) *GatewayData {
	ret := &GatewayData{}
	mdIn, _ := metadata.FromIncomingContext(ctx)
	s, _ := mdIn[GatewayMDataKey]
	if len(s) > 0 {
		_ = sonic.UnmarshalString(s[0], ret)
	}
	return ret
}
