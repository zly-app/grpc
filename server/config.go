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
	// 网关bind地址
	defGatewayBind = ":8080"
	// 关闭前等待时间, 单位秒
	defCloseWait = 3
	// 心跳时间
	defHeartbeatTime = 20
	// 最小心跳时间
	defMinHeartbeatTime = 3
	// 是否启用请求数据校验
	defReqDataValidate = true
	// 是否对请求数据校验所有字段
	defReqDataValidateAllField = false
)

// grpc服务配置
type ServerConfig struct {
	Bind                          string // bind地址
	GatewayBind                   string // 网关bind地址
	CloseWait                     int    // 关闭前等待时间, 单位秒
	HeartbeatTime                 int    // 心跳时间, 单位秒
	ReqDataValidate               bool   // 是否启用请求数据校验
	ReqDataValidateAllField       bool   // 是否对请求数据校验所有字段. 如果设为true, 会对所有字段校验并返回所有的错误. 如果设为false, 校验错误会立即返回.
	SendDetailedErrorInProduction bool   // 在生产环境发送详细的错误到客户端. 如果设为 false, 在生产环境且错误状态码为 Unknown, 则会返回 service internal error 给客户端.
	TLSCertFile                   string // tls公钥文件路径
	TLSKeyFile                    string // tls私钥文件路径
}

func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		HeartbeatTime:           defHeartbeatTime,
		ReqDataValidate:         defReqDataValidate,
		ReqDataValidateAllField: defReqDataValidateAllField,
	}
}

func (conf *ServerConfig) Check() error {
	if conf.Bind == "" {
		conf.Bind = defBind
	}
	if conf.GatewayBind == "" {
		conf.GatewayBind = defGatewayBind
	}
	if conf.CloseWait < 1 {
		conf.CloseWait = defCloseWait
	}
	if conf.HeartbeatTime < defMinHeartbeatTime {
		conf.HeartbeatTime = defMinHeartbeatTime
	}
	return nil
}
