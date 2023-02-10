package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/zly-app/zapp/core"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	GatewayMetadataMethod  = "gw.method"
	GatewayMetadataPath    = "gw.path-bin"
	GatewayMetadataHeaders = "gw.headers-bin"
	GatewayMetadataIP      = "gw.ip-bin"
	GatewayMetadataParams  = "gw.params-bin"
)

type GrpcHttpGatewayHandler = func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error

type Gateway struct {
	app                 core.IApp
	httpBind            string // 网关bind
	httpGatewayHandlers []GrpcHttpGatewayHandler
	server              *http.Server
}

func NewGateway(app core.IApp, httpBind string) *Gateway {
	return &Gateway{
		app:      app,
		httpBind: httpBind,
	}
}

func (g *Gateway) RegistryHttpGatewayHandler(hs ...GrpcHttpGatewayHandler) {
	for _, h := range hs {
		g.httpGatewayHandlers = append(g.httpGatewayHandlers, h)
	}
}

func (g *Gateway) StartGateway(serverPort int, tlsCertFile, tlsDomain string) error {
	gatewayListener, err := net.Listen("tcp", g.httpBind)
	if err != nil {
		return err
	}

	g.app.Info("grpc网关客户端正在连接")
	conn, err := g.makeGatewayConn(serverPort, tlsCertFile, tlsDomain)
	if err != nil {
		return err
	}

	gwMux := runtime.NewServeMux(runtime.WithMetadata(gatewayMetadataAnnotator))
	for _, h := range g.httpGatewayHandlers {
		err = h(context.Background(), gwMux, conn)
		if err != nil {
			return fmt.Errorf("注册grpc网关handler失败: %v", err)
		}
	}
	g.server = &http.Server{
		Addr:    g.httpBind,
		Handler: gwMux,
	}
	g.app.Info("正在启动grpc网关服务", zap.String("bind", gatewayListener.Addr().String()))
	return g.server.Serve(gatewayListener)
}

func (g *Gateway) Close() {
	err := g.server.Close()
	if err != nil {
		g.app.Error("关闭grpc网关服务失败", zap.Error(err))
	}
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

func (g *Gateway) makeGatewayConn(serverPort int, tlsCertFile, tlsDomain string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(g.app.BaseContext(), time.Second)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithBlock(), // 等待连接成功. 注意, 这个不要作为配置项, 因为要返回已连接完成的conn, 所以它是必须的.
	}

	if tlsCertFile != "" {
		tc, err := credentials.NewClientTLSFromFile(tlsCertFile, tlsDomain)
		if err != nil {
			return nil, fmt.Errorf("grpc网关客户端加载tls文件失败: %v", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(tc))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials())) // 不安全连接
	}

	target := fmt.Sprintf("localhost:%v", serverPort)
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc网关客户端连接失败: %v", err)
	}
	return conn, nil
}
