/*
-------------------------------------------------
   Author :       zlyuancn
   date：         2021/1/23
   Description :
-------------------------------------------------
*/

package server

const (
	// bind地址
	defBind = ":3000"
	// 心跳时间
	defHeartbeatTime = 20
	// 最小心跳时间
	defMinHeartbeatTime = 1
	// 是否设置请求日志等级设为info
	defReqLogLevelIsInfo = true
	// 是否设置响应日志等级设为info
	defRspLogLevelIsInfo = true
	// 是否启用OpenTrace
	defEnableOpenTrace = true
	// 是否启用请求数据校验
	defReqDataValidate = true
	// 是否对请求数据校验所有字段
	defReqDataValidateAllField = false
)

// grpc服务配置
type ServerConfig struct {
	Bind                          string // bind地址
	HeartbeatTime                 int    // 心跳时间, 单位秒, 默认20
	EnableOpenTrace               bool   // 是否启用OpenTrace
	ReqLogLevelIsInfo             bool   // 是否设置请求日志等级设为info
	RspLogLevelIsInfo             bool   // 是否设置响应日志等级设为info
	ReqDataValidate               bool   // 是否启用请求数据校验
	ReqDataValidateAllField       bool   // 是否对请求数据校验所有字段. 如果设为true, 会对所有字段校验并返回所有的错误. 如果设为false, 校验错误会立即返回.
	SendDetailedErrorInProduction bool   // 在生产环境发送详细的错误到客户端. 如果设为 true, 在非Debug环境且错误状态码为 Unknown, 则会返回 service internal error 给客户端.
}

func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		HeartbeatTime:           defHeartbeatTime,
		EnableOpenTrace:         defEnableOpenTrace,
		ReqLogLevelIsInfo:       defReqLogLevelIsInfo,
		RspLogLevelIsInfo:       defRspLogLevelIsInfo,
		ReqDataValidate:         defReqDataValidate,
		ReqDataValidateAllField: defReqDataValidateAllField,
	}
}

func (conf *ServerConfig) Check() error {
	if conf.Bind == "" {
		conf.Bind = defBind
	}
	if conf.HeartbeatTime < defMinHeartbeatTime {
		conf.HeartbeatTime = defMinHeartbeatTime
	}
	return nil
}
