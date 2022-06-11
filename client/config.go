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
	defBalance = balance.WeightConsistentHash
	// 连接超时
	defDialTimeout = 5000
	// 是否启用不安全的连接
	defInsecureDial = true
	// 是否启用OpenTrace
	defEnableOpenTrace = true
	// conn池大小
	defConnPoolSize = 1
	// conn池最大大小
	defMaxConnPoolSize = 10
	// 自动释放空闲conn间隔时间
	defAutoReleaseConnInterval = 60
	// 等待conn时间
	defWaitConnTime = 5000
)

// grpc客户端配置
type ClientConfig struct {
	Address                 string // 链接地址, 多个地址用英文逗号分隔
	Registry                string // 注册器, 支持 static, 默认为 static
	Balance                 string // 均衡器, 支持 round_robin, weight_random, weight_hash, weight_consistent_hash. 默认为 weight_consistent_hash
	DialTimeout             int    // 连接超时, 单位毫秒, 默认为 5000
	InsecureDial            bool   // 是否启用不安全的连接, 如果没有设置tls必须开启
	EnableOpenTrace         bool   // 是否启用OpenTrace
	ReqLogLevelIsInfo       bool   // 是否将请求日志等级设为info
	RspLogLevelIsInfo       bool   // 是否将响应日志等级设为info
	ConnPoolSize            int    // conn池大小, 表示对每个服务节点最少开启多少个链接
	MaxConnPoolSize         int    // conn池最大大小, 表示对每个服务节点最多开启多少个链接
	AutoReleaseConnInterval int    // 自动释放空闲conn间隔时间, 单位秒
	WaitConnTime            int    // 等待conn时间, 单位毫秒, 表示在conn池中获取一个conn的最大等待时间, -1表示一直等待直到有可用池
	ProxyAddress            string // 代理地址. 支持 socks5, socks5h. 示例: socks5://127.0.0.1:1080
	ProxyUser               string // 代理用户名
	ProxyPasswd             string // 代理用户密码
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
	if conf.ConnPoolSize < 1 {
		conf.ConnPoolSize = defConnPoolSize
	}
	if conf.MaxConnPoolSize < 1 {
		conf.MaxConnPoolSize = defMaxConnPoolSize
	}
	if conf.MaxConnPoolSize < conf.ConnPoolSize {
		conf.MaxConnPoolSize = conf.ConnPoolSize
	}
	if conf.AutoReleaseConnInterval < 1 {
		conf.AutoReleaseConnInterval = defAutoReleaseConnInterval
	}
	if conf.WaitConnTime < 0 {
		conf.WaitConnTime = -1
	} else if conf.WaitConnTime == 0 {
		conf.WaitConnTime = defWaitConnTime
	}
	return nil
}
