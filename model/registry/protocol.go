// Package registry 是模型层 —— 服务注册状态与注册中心线缆协议的所有者。
//
// 拥有：注册状态、字段级 TTL/续租规则、Redis key 协议（被注册侧与发现侧共享的共享模型）。
// 提供：StaticTable / RedisRegistry / Directory / ParseAddress / RegServer / RegSignal。
// 允许调用：foundation/addr、foundation/port、foundation/resolver。
// 禁止：让发现侧直翻本包内部状态；横向访问只经 port.StaticTable / port.Registry。
//
// 唯一施放入口：Register / Unregister 内原子完成「状态生效 + 通知发出」，
// 不允许外部只写状态不发信号。发现侧只读，不做任何删除或过期判定。
package registry

import (
	"strings"
)

const (
	TypeStatic = "static"
	TypeRedis  = "redis"
)

const (
	// RegistryExpire 是注册有效期（秒），由 Redis 字段级 TTL 保证；
	// ReRegInterval 是续租间隔（秒），续租即重设该 TTL。
	RegistryExpire = 30
	ReRegInterval  = 10
	// ReDiscoverInterval 是发现侧主动重发现间隔（秒），也是通知失败时的兜底。
	ReDiscoverInterval = 30
)

const (
	KeyServerReg = "grpc:server:reg:"    // 服务注册地址（hash，field 为 endpoint）
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

// RegKey / RegField / SignalKey 是注册中心与发现器共享的线缆协议。

// RegKey 返回服务注册 hash 的 key。
func RegKey(serverName string) string { return KeyServerReg + serverName }

// RegField 返回实例在注册 hash 中的字段名，即实例唯一标识 endpoint。
// endpoint 的唯一性由「同一机器的同一端口不可能被两个进程监听」物理保证。
func RegField(endpoint string) string { return strings.TrimSpace(endpoint) }

// SignalKey 返回服务注册信号的 Pub/Sub 通道。
func SignalKey(serverName string) string {
	return KeyRegSignal + serverName
}

// RegServer 是注册在 Redis hash 中的实例记录。
//
// Endpoint 既作 hash field 也作实例唯一标识；Name 进入一致性 hash 环，
// 必须跨重启稳定（默认 "<PublishName>@<endpoint>"，无 PublishName 时即 endpoint）。
// 有效期不落在值里：由 Redis 的字段级 TTL 表达，避免跨机时钟漂移导致误判。
type RegServer struct {
	Name     string
	Endpoint string
	Weight   uint16
}

// RegSignal 是注册状态变更通知；状态与信号由同一个入口发出。
type RegSignal struct {
	Reg     *RegServer
	IsUnReg bool
}
