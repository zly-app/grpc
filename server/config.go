/*
-------------------------------------------------
   Author :       zlyuancn
   date：         2021/1/23
   Description :
-------------------------------------------------
*/

package server

import (
	"runtime"
)

const (
	// bind地址
	defBind = ":3000"
	// http bind地址
	defHttpBind = ":8080"
	// 心跳时间
	defHeartbeatTime = 20
	// 最小心跳时间
	defMinHeartbeatTime = 3
	// 是否设置请求日志等级设为info
	defReqLogLevelIsInfo = true
	// 是否设置响应日志等级设为info
	defRspLogLevelIsInfo = true
	// 是否启用请求数据校验
	defReqDataValidate = true
	// 是否对请求数据校验所有字段
	defReqDataValidateAllField = false
	// 同时处理请求的goroutine数
	defThreadCount = 0
	// 最大请求等待队列大小
	defMaxReqWaitQueueSize = 10000
)

// grpc服务配置
type ServerConfig struct {
	Bind                          string // bind地址
	HttpBind                      string // http bind地址
	HeartbeatTime                 int    // 心跳时间, 单位秒
	ReqLogLevelIsInfo             bool   // 是否设置请求日志等级设为info
	RspLogLevelIsInfo             bool   // 是否设置响应日志等级设为info
	ReqDataValidate               bool   // 是否启用请求数据校验
	ReqDataValidateAllField       bool   // 是否对请求数据校验所有字段. 如果设为true, 会对所有字段校验并返回所有的错误. 如果设为false, 校验错误会立即返回.
	SendDetailedErrorInProduction bool   // 在生产环境发送详细的错误到客户端. 如果设为 false, 在生产环境且错误状态码为 Unknown, 则会返回 service internal error 给客户端.
	// 同时处理请求的goroutine数, 设为0时取逻辑cpu数*2, 设为负数时不作任何限制, 每个请求由独立的线程执行
	ThreadCount int
	// 最大请求等待队列大小
	//
	// 只有 ThreadCount >= 0 时生效.
	// 启动时创建一个指定大小的任务队列, 触发产生的请求会放入这个队列, 队列已满时新触发的请求会返回错误
	MaxReqWaitQueueSize int
	TLSCertFile         string // tls公钥文件路径
	TLSKeyFile          string // tls私钥文件路径
	TLSDomain           string // tls签发域名
}

func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		HeartbeatTime:           defHeartbeatTime,
		ReqLogLevelIsInfo:       defReqLogLevelIsInfo,
		RspLogLevelIsInfo:       defRspLogLevelIsInfo,
		ReqDataValidate:         defReqDataValidate,
		ReqDataValidateAllField: defReqDataValidateAllField,
		ThreadCount:             defThreadCount,
	}
}

func (conf *ServerConfig) Check() error {
	if conf.Bind == "" {
		conf.Bind = defBind
	}
	if conf.HttpBind == "" {
		conf.HttpBind = defHttpBind
	}
	if conf.HeartbeatTime < defMinHeartbeatTime {
		conf.HeartbeatTime = defMinHeartbeatTime
	}
	if conf.ThreadCount == 0 {
		conf.ThreadCount = runtime.NumCPU() * 2
	}
	if conf.ThreadCount < 0 {
		conf.ThreadCount = -1
	}
	if conf.MaxReqWaitQueueSize < 1 {
		conf.MaxReqWaitQueueSize = defMaxReqWaitQueueSize
	}
	return nil
}
