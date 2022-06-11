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
	defDialTimeout = 5
	// 是否启用不安全的连接
	defInsecureDial = true
	// 是否启用OpenTrace
	defEnableOpenTrace = true
	// 是否设置请求日志等级设为info
	defReqLogLevelIsInfo = true
	// 是否设置响应日志等级设为info
	defRspLogLevelIsInfo = true
	// conn池大小
	defConnPoolSize = 5
	// conn池最大大小
	defMaxConnPoolSize = 20
	// 当连接池中的连接耗尽的时候一次同时获取的连接数
	defAcquireIncrement = 5
	// conn空闲时间
	defConnIdleTime = 60
	// 自动释放空闲conn间隔时间
	defAutoReleaseConnInterval = 10
	// 最大等待conn数量
	defMaxWaitConnSize = 1000
	// 等待conn时间
	defWaitConnTime = 5
)

// grpc客户端配置
type ClientConfig struct {
	Address                 string // 链接地址
	Registry                string // 注册器, 支持 static
	Balance                 string // 均衡器, 支持 round_robin, weight_random, weight_hash, weight_consistent_hash
	DialTimeout             int    // 连接超时, 单位秒
	InsecureDial            bool   // 是否启用不安全的连接, 如果没有设置tls必须开启
	EnableOpenTrace         bool   // 是否启用OpenTrace
	ReqLogLevelIsInfo       bool   // 是否将请求日志等级设为info
	RspLogLevelIsInfo       bool   // 是否将响应日志等级设为info
	ConnPoolSize            int    // conn池大小, 表示对每个服务节点最少开启多少个链接
	MaxConnPoolSize         int    // conn池最大大小, 表示对每个服务节点最多开启多少个链接
	AcquireIncrement        int    // 当连接池中的连接耗尽的时候一次同时获取的连接数
	ConnIdleTime            int    // conn空闲时间, 单位秒, 当conn空闲达到一定时间则被标记为可释放
	AutoReleaseConnInterval int    // 自动释放空闲conn检查间隔时间, 单位秒
	MaxWaitConnSize         int    // 最大等待conn数量, 当连接池满后, 新建连接将等待池中连接释放后才可以继续, 等待的数量超出阈值则返回错误
	WaitConnTime            int    // 等待conn时间, 单位秒, 表示在conn池中获取一个conn的最大等待时间, -1表示一直等待直到有可用池
	ProxyAddress            string // 代理地址. 支持 socks5, socks5h. 示例: socks5://127.0.0.1:1080
	ProxyUser               string // 代理用户名
	ProxyPasswd             string // 代理用户密码
}

func NewClientConfig() *ClientConfig {
	return &ClientConfig{
		InsecureDial:      defInsecureDial,
		EnableOpenTrace:   defEnableOpenTrace,
		ReqLogLevelIsInfo: defReqLogLevelIsInfo,
		RspLogLevelIsInfo: defRspLogLevelIsInfo,
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
	if conf.AcquireIncrement < 1 {
		conf.AcquireIncrement = defAcquireIncrement
	}
	if conf.ConnIdleTime < 1 {
		conf.ConnIdleTime = defConnIdleTime
	}
	if conf.AutoReleaseConnInterval < 1 {
		conf.AutoReleaseConnInterval = defAutoReleaseConnInterval
	}
	if conf.MaxWaitConnSize < 1 {
		conf.MaxWaitConnSize = defMaxWaitConnSize
	}
	if conf.WaitConnTime < 0 {
		conf.WaitConnTime = -1
	} else if conf.WaitConnTime == 0 {
		conf.WaitConnTime = defWaitConnTime
	}
	return nil
}
