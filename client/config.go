package client

import (
	"github.com/zly-app/grpc/balance"
)

const (
	// 服务地址
	defAddress = "localhost:3000"
	// 均衡器
	defBalance = balance.WeightConsistentHash

	// 初始化时等待第一个链接
	defWaitFirstConn = false
	// 最小闲置
	defMinIdle = 2
	// 最大闲置
	defMaxIdle = defMinIdle * 2
	// 最大活跃连接数
	defMaxActive = 10
	// 批次增量
	defBatchIncrement = defMinIdle * 2
	// 批次缩容
	defBatchShrink = defBatchIncrement
	// 空闲链接超时时间
	defIdleTimeout = 3600
	// 等待获取连接的超时时间
	defWaitTimeout = 5
	// 最大等待conn的数量
	defMaxWaitConnCount = 2000
	// 连接超时
	defConnectTimeout = 5
	// 一个连接最大存活时间
	defMaxConnLifetime = 3600
	// 检查空闲间隔, 包含最小空闲数, 最大空闲数, 空闲链接超时
	defCheckIdleInterval = 5
)

// grpc客户端配置
type ClientConfig struct {
	Address           string // 服务地址, 参考 https://github.com/zly-app/grpc/tree/master/discover
	Balance           string // 均衡器, 支持 round_robin, weight_random, weight_hash, weight_consistent_hash

	WaitFirstConn     bool // 初始化时等待第一个链接
	MinIdle           int  // 最小闲置
	MaxIdle           int  // 最大闲置
	MaxActive         int  // 最大活跃连接数, 小于1表示不限制
	BatchIncrement    int  // 批次增量, 当conn不够时, 一次性最多申请多少个链接
	BatchShrink       int  // 批次缩容, 当conn太多时(超过最大闲置), 一次性最多释放多少个链接
	IdleTimeout       int  // 空闲链接超时时间, 单位秒, 如果一个连接长时间未使用将被视为连接无效, 小于1表示永不超时
	WaitTimeout       int  // 等待获取连接的超时时间, 单位秒
	MaxWaitConnCount  int  // 最大等待conn的数量, 小于1表示不限制
	ConnectTimeout    int  // 连接超时, 单位秒
	MaxConnLifetime   int  // 一个连接最大存活时间, 单位秒, 小于1表示不限制
	CheckIdleInterval int  // 检查空闲间隔, 单位秒

	ProxyAddress string // 代理地址. 支持 socks5, socks5h. 示例: socks5://127.0.0.1:1080 socks5://user:pwd@127.0.0.1:1080
	TLSCertFile  string // tls公钥文件路径
	TLSDomain    string // tls签发域名
}

func NewClientConfig() *ClientConfig {
	return &ClientConfig{
		WaitFirstConn:    defWaitFirstConn,
		MaxActive:        defMaxActive,
		IdleTimeout:      defIdleTimeout,
		MaxWaitConnCount: defMaxWaitConnCount,
		MaxConnLifetime:  defMaxConnLifetime,
	}
}

func (conf *ClientConfig) Check() error {
	if conf.Address == "" {
		conf.Address = defAddress
	}
	if conf.Balance == "" {
		conf.Balance = defBalance
	}

	if conf.MinIdle < 1 {
		conf.MinIdle = defMinIdle
	}
	if conf.MaxIdle < 1 {
		conf.MaxIdle = defMaxIdle
	}
	if conf.MaxIdle < conf.MinIdle {
		conf.MaxIdle = conf.MinIdle * 2
	}
	if conf.BatchIncrement < 1 {
		conf.BatchIncrement = conf.MinIdle
	}
	if conf.BatchIncrement > conf.MaxIdle {
		conf.BatchIncrement = conf.MaxIdle
	}
	if conf.BatchShrink < 1 {
		conf.BatchShrink = defBatchShrink
	}
	if conf.IdleTimeout < 1 {
		conf.IdleTimeout = 0
	}
	if conf.WaitTimeout < 1 {
		conf.WaitTimeout = defWaitTimeout
	}
	if conf.MaxWaitConnCount < 1 {
		conf.MaxWaitConnCount = 0
	}
	if conf.ConnectTimeout < 1 {
		conf.ConnectTimeout = defConnectTimeout
	}
	if conf.MaxConnLifetime < 1 {
		conf.MaxConnLifetime = 0
	}
	if conf.CheckIdleInterval < 1 {
		conf.CheckIdleInterval = defCheckIdleInterval
	}
	return nil
}
