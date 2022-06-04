package client

import (
	"github.com/zly-app/grpc/balance"
	"github.com/zly-app/grpc/registry/static"
)

const (
	// 连接地址
	defAddress = "localhost:3000"
	// 注册器
	defRegistry = static.Name
	// 均衡器
	defBalance = balance.RoundRobin
	// 连接超时
	defDialTimeout = 5000
	// 是否启用不安全的连接
	defInsecureDial = true
	// 是否启用OpenTrace
	defEnableOpenTrace = true
)

// grpc客户端配置
type ClientConfig struct {
	Address           string // 链接地址
	Registry          string // 注册器, 默认为 static
	Balance           string // 均衡器, 默认为 round_robin
	DialTimeout       int    // 连接超时, 单位毫秒, 默认为 5000
	InsecureDial      bool   // 是否启用不安全的连接, 如果没有设置tls必须开启
	EnableOpenTrace   bool   // 是否启用OpenTrace
	ReqLogLevelIsInfo bool   // 是否将请求日志等级设为info
	RspLogLevelIsInfo bool   // 是否将响应日志等级设为info
}

func NewClientConfig() *ClientConfig {
	return &ClientConfig{
		InsecureDial:    defInsecureDial,
		EnableOpenTrace: defEnableOpenTrace,
	}
}

func (conf *ClientConfig) Check() error {
	if conf.Address == "" {
		conf.Address = defAddress
	}
	if conf.Registry == "" {
		conf.Registry = defRegistry
	}
	if conf.Balance == "" {
		conf.Balance = defBalance
	}
	if conf.DialTimeout < 1 {
		conf.DialTimeout = defDialTimeout
	}
	return nil
}
