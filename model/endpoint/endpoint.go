// Package endpoint 是模型层 —— 服务端点的来源规格与注册协议的所有者。
//
// 拥有：地址来源规格（static / 直连 / redis）、来源字符串解析、注册记录、变更信号、
// Redis key 布局与有效期常量。注册侧与发现侧都只依赖本包，彼此不互相 import。
// 提供：Spec / Source / ParseSource / Record / Signal / RegKey / SignalKey / Field / RecordName。
// 允许调用：无（纯值对象与协议常量）。
// 禁止：建连、读写 Redis、判定「该连谁」。
package endpoint

import (
	"fmt"
	"strings"
)

// 地址来源类型：Spec.Type。
const (
	TypeStatic = "static"
	TypeRedis  = "redis"
)

const (
	// RecordTTL 是注册记录的有效期（秒），由 Redis 的字段级 TTL 保证。
	RecordTTL = 30
	// LeaseInterval 是续租间隔（秒），续租即重设该 TTL。
	LeaseInterval = 10
	// ReDiscoverInterval 是发现侧主动重发现的间隔（秒），也是信号丢失时的兜底。
	ReDiscoverInterval = 30
)

const (
	// KeyRegPrefix 是服务注册记录的 hash key 前缀（field 为 endpoint）。
	KeyRegPrefix = "grpc:server:reg:"
	// KeySignalPrefix 是注册状态变更通道的前缀。
	KeySignalPrefix = "grpc:server:signal:"
)

// Spec 是注册/发现目录认识的最小来源规格：Type 选实现，Component 是该实现的寻址参数。
type Spec struct {
	Type      string // static | redis
	Component string // Type=redis 时为 redis 组件名（components.redis.<name>）
}

// Source 是配置里那一行来源字符串（如 "redis://grpc_registry"）的解析结果。
//
// 语法只有下面几种，其它一律报错——不猜、不降级：
//
//	""                      → static（服务端不对外公告，客户端回退默认地址）
//	"static"                → static
//	"host:port[,host:port]" → static + 直连地址
//	"redis://<组件名>"      → redis + components.redis.<组件名>
//	"static://a,b"          → 显式 static + 直连地址
type Source struct {
	Type      string
	Component string
	Addresses []string
}

// Spec 丢掉地址列表，只保留目录需要的部分。
func (s Source) Spec() Spec {
	return Spec{Type: s.Type, Component: s.Component}
}

// ParseSource 严格解析来源字符串：未知 scheme、redis 缺组件名都在这里报错，
// 让配置错误停在启动校验期，而不是等到拨号或「悄悄连 localhost」。
func ParseSource(raw string) (Source, error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.EqualFold(s, TypeStatic) {
		return Source{Type: TypeStatic}, nil
	}

	if k := strings.Index(s, "://"); k >= 0 {
		typ := strings.ToLower(strings.TrimSpace(s[:k]))
		val := strings.TrimSpace(s[k+3:])
		switch typ {
		case TypeStatic:
			return Source{Type: TypeStatic, Addresses: splitAddresses(val)}, nil
		case TypeRedis:
			if val == "" {
				return Source{}, fmt.Errorf("redis 来源缺少组件名，应为 redis://<components.redis 组件名>")
			}
			if strings.ContainsAny(val, ",/ \t") {
				return Source{}, fmt.Errorf("redis 组件名不能包含空白、逗号或斜杠: %q", val)
			}
			return Source{Type: TypeRedis, Component: val}, nil
		default:
			return Source{}, fmt.Errorf("未知的来源类型 %q：只支持 %q、\"<host:port>[,<host:port>]\" 或 \"redis://<组件名>\"", typ, TypeStatic)
		}
	}

	return Source{Type: TypeStatic, Addresses: splitAddresses(s)}, nil
}

func splitAddresses(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Record 是注册中心里的实例记录（线缆格式）。
//
// Endpoint 既是 hash field 也是实例唯一标识；Name 进入一致性 hash 环，
// 必须跨重启稳定。有效期不落在值里：由 Redis 的字段级 TTL 表达，
// 避免注册方与发现方的跨机时钟漂移导致误判。
type Record struct {
	Name     string
	Endpoint string
	Weight   uint16
}

// Signal 是一次注册状态变更通知；IsUnReg 为真表示该实例已注销。
type Signal struct {
	Record  *Record
	IsUnReg bool
}

// RegKey 返回服务注册 hash 的 key。
func RegKey(serverName string) string { return KeyRegPrefix + serverName }

// SignalKey 返回服务注册信号的 Pub/Sub 通道。
func SignalKey(serverName string) string { return KeySignalPrefix + serverName }

// Field 返回实例在注册 hash 中的字段名，即实例唯一标识 endpoint。
// 唯一性由「同一机器的同一端口不可能被两个进程监听」物理保证。
func Field(ep string) string { return strings.TrimSpace(ep) }

// RecordName 生成进入一致性 hash 环的节点名：以 endpoint 结尾保证同实例稳定，
// publishName 只作可读前缀，不参与唯一性。
func RecordName(publishName, ep string) string {
	ep = Field(ep)
	publishName = strings.TrimSpace(publishName)
	if publishName == "" || publishName == ep {
		return ep
	}
	return publishName + "@" + ep
}
