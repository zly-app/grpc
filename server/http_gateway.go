package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type GrpcHttpGatewayHandler = func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error

func (g *GRpcServer) RegistryHttpGatewayHandler(hs ...GrpcHttpGatewayHandler) {
	for _, h := range hs {
		g.httpGatewayHandlers = append(g.httpGatewayHandlers, h)
	}
}

func (g *GRpcServer) StartGateway() error {
	listener, err := net.Listen("tcp", g.conf.Bind)
	if err != nil {
		return err
	}
	gatewayListener, err := net.Listen("tcp", g.conf.HttpBind)
	if err != nil {
		return err
	}

	g.app.Info("正在启动grpc服务", zap.String("bind", listener.Addr().String()))
	go func() {
		err := g.server.Serve(listener)
		if err != nil {
			g.app.Error("grpc服务启动失败", zap.Error(err))
		}
		g.app.Info("grpc服务启动成功", zap.String("bind", listener.Addr().String()))
	}()

	serverPort := listener.Addr().(*net.TCPAddr).Port
	g.app.Info("网关客户端正在连接")
	conn, err := g.makeGatewayConn(serverPort)
	if err != nil {
		return err
	}

	gwMux := runtime.NewServeMux()
	for _, h := range g.httpGatewayHandlers {
		err = h(context.Background(), gwMux, conn)
		if err != nil {
			return fmt.Errorf("注册网关handler失败: %v", err)
		}
	}
	gwServer := &http.Server{
		Addr:    g.conf.HttpBind,
		Handler: gwMux,
	}

	g.app.Info("正在启动grpc网关服务", zap.String("bind", gatewayListener.Addr().String()))
	return gwServer.Serve(gatewayListener)
}

func (g *GRpcServer) makeGatewayConn(port int) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(g.app.BaseContext(), time.Second)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithBlock(), // 等待连接成功. 注意, 这个不要作为配置项, 因为要返回已连接完成的conn, 所以它是必须的.
	}

	if g.conf.TLSCertFile != "" {
		tc, err := credentials.NewClientTLSFromFile(g.conf.TLSCertFile, g.conf.TLSDomain)
		if err != nil {
			return nil, fmt.Errorf("grpc网关客户端加载tls文件失败: %v", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(tc))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials())) // 不安全连接
	}

	target := fmt.Sprintf("localhost:%v", port)
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc网关客户端连接失败: %v", err)
	}
	return conn, nil
}
