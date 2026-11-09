// Package registry 是模型层 —— 服务注册状态与注册中心线缆协议的所有者。
//
// 拥有：注册状态、TTL/续租规则、Redis key 协议（被注册侧与发现侧共享的共享模型）。
// 提供：StaticTable / RedisRegistry / Directory / ParseAddress / RegServer / RegSignal。
// 允许调用：foundation/addr、foundation/port、foundation/resolver。
// 禁止：让发现侧直翻本包内部状态；横向访问只经 port.StaticTable / port.Registry。
//
// 唯一施放入口：Register / Unregister 内原子完成「状态生效 + 通知发出」，
// 不允许外部只写状态不发信号。
package registry

import (
	"strings"

	"github.com/spf13/cast"
)

const (
	TypeStatic = "static"
	TypeRedis  = "redis"
)

const (
	// RegistryExpire 是注册有效期（秒），ReRegInterval 是续租间隔（秒）。
	RegistryExpire = 30
	ReRegInterval  = 10
	// DelRegDataThanTimeOverdue 是发现侧清理超长过期数据的阈值（秒）。
	DelRegDataThanTimeOverdue = 300
	// ReDiscoverInterval 是发现侧主动重发现间隔（秒），也是通知失败时的兜底。
	ReDiscoverInterval = 30
)

const (
	KeySeqIncr   = "grpc:server:seq:"    // 服务申请序号自增号
	KeyServerReg = "grpc:server:reg:"    // 服务注册地址
	KeyRegSignal = "grpc:server:signal:" // 注册信号通道
)

// AddressSpec 是注册地址解析结果；注册侧与发现侧共用同一套解析规则。
type AddressSpec struct {
	Type string
	Addr string
}

// ParseAddress 解析形如 "redis://name" / "localhost:3000" / "static" 的地址。
func ParseAddress(s string) AddressSpec {
	s = strings.TrimSpace(s)
	if s == "" || s == TypeStatic {
		return AddressSpec{Type: TypeStatic}
	}
	k := strings.Index(s, "://")
	if k == -1 {
		return AddressSpec{Type: TypeStatic, Addr: s}
	}
	typ := strings.ToLower(s[:k])
	if typ == "" {
		typ = TypeStatic
	}
	return AddressSpec{Type: typ, Addr: s[k+3:]}
}

// SeqKey / RegKey / RegField / SignalKey 是注册中心与发现器共享的线缆协议。
func SeqKey(serverName string) string { return KeySeqIncr + serverName }
func RegKey(serverName string) string { return KeyServerReg + serverName }
func RegField(seq int) string         { return cast.ToString(seq) }
func SignalKey(serverName string) string {
	return KeyRegSignal + serverName
}

// RegServer 是注册在 Redis hash 中的实例记录。
type RegServer struct {
	SeqNo    int
	Name     string
	Endpoint string
	Weight   uint16
	Deadline int64
}

// RegSignal 是注册状态变更通知；状态与信号由同一个入口发出。
type RegSignal struct {
	Reg     *RegServer
	IsUnReg bool
}
