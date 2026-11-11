// Package config 是模型层 —— 三份配置各自独占自己的字段与默认值/约束规则。
//
// 拥有：ServerConfig / ClientConfig / GatewayConfig 的字段与默认取值。
// 提供：NewXxxConfig（带默认值）、Check（约束修正）、配置 key 常量。
// 允许调用：model/balance（默认均衡器名）。禁止：读取 zapp 配置、判定「该启用谁」。
package config

import (
	"fmt"
	"strings"

	"github.com/zly-app/zapp/core"

	"github.com/zly-app/grpc/model/balance"
)

const (
	// ServiceTypeGRPC 是 zapp 服务类型，同时是 services.grpc.* 的配置根。
	ServiceTypeGRPC core.ServiceType = "grpc"
	// ServiceTypeGateway 是 HTTP 网关的服务类型。
	ServiceTypeGateway core.ServiceType = "grpc-gateway"
	// ComponentType 是 components.grpc.* 的配置根。
	ComponentType core.ComponentType = "grpc"
)

// ServerConfig 拥有服务端监听、TLS、校验、注册公告的规则。
type ServerConfig struct {
	Bind                          string
	HeartbeatTime                 int
	ReqDataValidate               bool
	ReqDataValidateAllField       bool
	SendDetailedErrorInProduction bool
	TLSCertFile                   string
	TLSKeyFile                    string

	RegistryAddress string
	PublishName     string
	PublishAddress  string
	PublishWeight   uint16
}

const (
	defServerBind          = ":3000"
	defHeartbeatTime       = 20
	defMinHeartbeatTime    = 3
	defReqDataValidate     = true
	defRegistryAddressType = "static"
	defWeight              = uint16(100)
)

// NewServerConfig 提供一份带默认值的服务端配置。
func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		HeartbeatTime:           defHeartbeatTime,
		ReqDataValidate:         defReqDataValidate,
		RegistryAddress:         defRegistryAddressType,
		PublishWeight:           defWeight,
		Bind:                    defServerBind,
		ReqDataValidateAllField: false,
	}
}

// Check 修正不满足约束的取值。
func (c *ServerConfig) Check() error {
	if c.Bind == "" {
		c.Bind = defServerBind
	}
	if c.HeartbeatTime < defMinHeartbeatTime {
		c.HeartbeatTime = defMinHeartbeatTime
	}
	if c.RegistryAddress == "" {
		c.RegistryAddress = defRegistryAddressType
	}
	if c.PublishWeight == 0 {
		c.PublishWeight = defWeight
	}
	return nil
}

// ClientConfig 拥有连接池、地址、均衡器、代理与 TLS 的规则。
type ClientConfig struct {
	Address string
	Balance string

	WaitFirstConn     bool
	MinIdle           int
	MaxIdle           int
	MaxActive         int
	BatchIncrement    int
	BatchShrink       int
	IdleTimeout       int
	WaitTimeout       int
	MaxWaitConnCount  int
	ConnectTimeout    int
	MaxConnLifetime   int
	CheckIdleInterval int

	ProxyAddress string
	TLSCertFile  string
	TLSDomain    string
}

const (
	defClientAddress     = "localhost:3000"
	defMinIdle           = 2
	defMaxIdle           = defMinIdle * 2
	defMaxActive         = 10
	defBatchShrink       = defMinIdle * 2
	defIdleTimeout       = 3600
	defWaitTimeout       = 5
	defMaxWaitConnCount  = 2000
	defConnectTimeout    = 5
	defMaxConnLifetime   = 3600
	defCheckIdleInterval = 5
)

// NewClientConfig 提供一份带默认值的客户端配置。
func NewClientConfig() *ClientConfig {
	return &ClientConfig{
		Address:          defClientAddress,
		Balance:          balance.WeightConsistentHash,
		MaxActive:        defMaxActive,
		IdleTimeout:      defIdleTimeout,
		MaxWaitConnCount: defMaxWaitConnCount,
		MaxConnLifetime:  defMaxConnLifetime,
	}
}

// Check 修正不满足约束的取值。
func (c *ClientConfig) Check() error {
	if c.Address == "" {
		c.Address = defClientAddress
	}
	if c.Balance == "" {
		c.Balance = balance.WeightConsistentHash
	}
	if c.MinIdle < 1 {
		c.MinIdle = defMinIdle
	}
	if c.MaxIdle < 1 {
		c.MaxIdle = defMaxIdle
	}
	if c.MaxIdle < c.MinIdle {
		c.MaxIdle = c.MinIdle * 2
	}
	if c.BatchIncrement < 1 {
		c.BatchIncrement = c.MinIdle
	}
	if c.BatchIncrement > c.MaxIdle {
		c.BatchIncrement = c.MaxIdle
	}
	if c.BatchShrink < 1 {
		c.BatchShrink = defBatchShrink
	}
	if c.IdleTimeout < 1 {
		c.IdleTimeout = 0
	}
	if c.WaitTimeout < 1 {
		c.WaitTimeout = defWaitTimeout
	}
	if c.MaxWaitConnCount < 1 {
		c.MaxWaitConnCount = 0
	}
	if c.ConnectTimeout < 1 {
		c.ConnectTimeout = defConnectTimeout
	}
	if c.MaxConnLifetime < 1 {
		c.MaxConnLifetime = 0
	}
	if c.CheckIdleInterval < 1 {
		c.CheckIdleInterval = defCheckIdleInterval
	}
	return nil
}

// HashKeyConfig 拥有网关 hashKey 的取值规则。
//
// 语义：网关把某个请求头的值作为一致性 hash 的 key，交给负载均衡器选实例。
// ByHeader 是默认来源；Rules 按 path 覆盖它，命中且 ByHeader 为空的规则表示
// 该 path 显式不参与 hash。path 取 grpc-gateway 注册的路径模板，不是原始 URL。
type HashKeyConfig struct {
	ByHeader string
	Rules    []HashKeyRule
}

// HashKeyRule 覆盖单条 path 的 hashKey 来源；规则本体在 model/hashkey。
type HashKeyRule struct {
	Path     string
	ByHeader string
}

// Check 规范空白并拒绝不合法的规则：配置错误必须在启动期暴露，不能拖到运行期静默失效。
func (c *HashKeyConfig) Check() error {
	c.ByHeader = strings.TrimSpace(c.ByHeader)
	if c.ByHeader != "" && !validHeaderName(c.ByHeader) {
		return fmt.Errorf("HashKey.ByHeader 不是合法的 HTTP 头名: %q", c.ByHeader)
	}
	seen := make(map[string]struct{}, len(c.Rules))
	for i := range c.Rules {
		r := &c.Rules[i]
		r.Path = strings.TrimSpace(r.Path)
		r.ByHeader = strings.TrimSpace(r.ByHeader)
		if r.Path == "" {
			return fmt.Errorf("HashKey.Rules[%d] 缺少 Path", i)
		}
		if !strings.HasPrefix(r.Path, "/") {
			return fmt.Errorf("HashKey.Rules[%d].Path 必须以 / 开头: %q", i, r.Path)
		}
		if r.ByHeader != "" && !validHeaderName(r.ByHeader) {
			return fmt.Errorf("HashKey.Rules[%d].ByHeader 不是合法的 HTTP 头名: %q", i, r.ByHeader)
		}
		if _, ok := seen[r.Path]; ok {
			return fmt.Errorf("HashKey.Rules 中 Path 重复: %q", r.Path)
		}
		seen[r.Path] = struct{}{}
	}
	return nil
}

// validHeaderName 按 RFC 7230 的 token 规则校验头名，挡住拼写错误与非法字符。
func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)):
		default:
			return false
		}
	}
	return true
}

// GatewayConfig 拥有网关监听、跨域与 hashKey 的规则。
type GatewayConfig struct {
	Bind         string
	CloseWait    int
	CorsAllowAll bool
	HashKey      HashKeyConfig
}

const (
	defGatewayBind      = ":8080"
	defGatewayCloseWait = 3
)

// NewGatewayConfig 提供一份带默认值的网关配置。
func NewGatewayConfig() *GatewayConfig {
	return &GatewayConfig{
		Bind:         defGatewayBind,
		CloseWait:    defGatewayCloseWait,
		CorsAllowAll: true,
	}
}

// Check 修正不满足约束的取值，并把不合法的 hashKey 规则判为错误。
func (c *GatewayConfig) Check() error {
	if c.Bind == "" {
		c.Bind = defGatewayBind
	}
	if c.CloseWait < 1 {
		c.CloseWait = defGatewayCloseWait
	}
	return c.HashKey.Check()
}
