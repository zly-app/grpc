/*
-------------------------------------------------
   Author :       zlyuancn
   date：         2021/1/23
   Description :
-------------------------------------------------
*/

package server

import (
	"github.com/zly-app/grpc/registry/static"
)

const (
	// bind地址
	defBind = ":3000"
	// 心跳时间
	defHeartbeatTime = 20
	// 最小心跳时间
	defMinHeartbeatTime = 3
	// 是否启用请求数据校验
	defReqDataValidate = true
	// 是否对请求数据校验所有字段
	defReqDataValidateAllField = false

	defRegistryType = static.Name
	defWeight       = 100
)

// grpc服务配置
type ServerConfig struct {
	Bind                          string // bind地址
	HeartbeatTime                 int    // 心跳时间, 单位秒
	ReqDataValidate               bool   // 是否启用请求数据校验
	ReqDataValidateAllField       bool   // 是否对请求数据校验所有字段. 如果设为true, 会对所有字段校验并返回所有的错误. 如果设为false, 校验错误会立即返回.
	SendDetailedErrorInProduction bool   // 在生产环境发送详细的错误到客户端. 如果设为 false, 在生产环境且错误状态码为 Unknown, 则会返回 service internal error 给客户端.
	TLSCertFile                   string // tls公钥文件路径
	TLSKeyFile                    string // tls私钥文件路径

	RegistryName   string // 注册器名称
	RegistryType   string // 注册器类型, 支持 static, redis
	PublishName    string // 公告名, 在注册中心中定义的名称, 如果为空则自动设为 PublishAddress
	PublishAddress string // 公告地址, 在注册中心中定义的地址, 客户端会根据这个地址连接服务端, 如果为空则自动设为 实例ip:BindPort
	PublishWeight  uint16 // 公告权重, 默认100
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
	if conf.HeartbeatTime < defMinHeartbeatTime {
		conf.HeartbeatTime = defMinHeartbeatTime
	}

	if conf.RegistryType == "" {
		conf.RegistryType = defRegistryType
	}
	if conf.PublishWeight == 0 {
		conf.PublishWeight = defWeight
	}
	return nil
}
