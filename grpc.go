// Package grpc 是根包门面 —— 只把组合根与值类型转发出去，不含任何逻辑。
//
// 装配入口：New(...) → Options() 传给 zapp.NewApp → 声明服务/取连接 → Run。
// 所有实现都在 app / adapter / usecase / model / foundation 五层里，根包不持有状态。
package grpc

import (
	"context"

	"google.golang.org/grpc"

	"github.com/zly-app/grpc/app"
	"github.com/zly-app/grpc/foundation/ctxkey"
	"github.com/zly-app/grpc/foundation/wire"
	"github.com/zly-app/grpc/model/code"
)

// App 是唯一装配体。
type App = app.App

// Option 是装配期选项。
type Option = app.Option

// New 创建唯一组合根。
func New(opts ...Option) *App { return app.New(opts...) }

// WithServer / WithGateway 启用对应的 zapp 服务。
func WithServer() Option  { return app.WithServer() }
func WithGateway() Option { return app.WithGateway() }

// WithServerHooks / WithClientHooks / WithAllClientHooks 注册拦截器。
func WithServerHooks(hs ...ServerHook) Option { return app.WithServerHooks(hs...) }
func WithClientHooks(name string, hs ...ClientHook) Option {
	return app.WithClientHooks(name, hs...)
}
func WithAllClientHooks(hs ...ClientHook) Option { return app.WithAllClientHooks(hs...) }

// WithoutErrCodeHook 关闭错误码规则的默认装配。
func WithoutErrCodeHook() Option { return app.WithoutErrCodeHook() }

// 常用类型别名。
type (
	ServerHook          = grpc.UnaryServerInterceptor
	ClientHook          = grpc.UnaryClientInterceptor
	ServiceRegistrar    = grpc.ServiceRegistrar
	ClientConnInterface = grpc.ClientConnInterface
	Code                = code.Code
	GatewayData         = ctxkey.GatewayData
)

// 错误码工具。
func Error(c Code, msg string) error                       { return code.Error(c, msg) }
func Errorf(c Code, format string, a ...interface{}) error { return code.Errorf(c, format, a...) }
func GetErrCode(err error) Code                            { return code.GetErrCode(err) }

// 请求选择器（横切上下文经底座传播）。
func WithTarget(target string) grpc.CallOption { return ctxkey.WithTarget(target) }
func WithHashKey(key string) grpc.CallOption   { return ctxkey.WithHashKey(key) }

// 自定义 metadata 工具。
const WrapMetadataPrefix = wire.WrapMetadataPrefix

func ServerExtractCustomData(ctx context.Context, key string) []string {
	return wire.ServerExtractCustomData(ctx, key)
}
func ClientInjectCustomData(ctx context.Context, key string, value []string) context.Context {
	return wire.ClientInjectCustomData(ctx, key, value)
}

// GetGatewayData 在被调方读取网关传来的请求上下文；取不到时返回空对象。
func GetGatewayData(ctx context.Context) *GatewayData {
	return ctxkey.GatewayDataFromIncoming(ctx)
}
