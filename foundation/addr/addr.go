// Package addr 是底座 —— 地址值对象与 gRPC resolver 属性的编解码机制。
//
// 拥有：无（纯值转换，不持有状态）。
// 提供：Info / Set / Get / Parse / BindPort。
// 允许调用：无上层。禁止：在这里判定「该连谁」——那是均衡器模型的规则。
package addr

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cast"
	"google.golang.org/grpc/resolver"
)

const (
	Scheme      = "grpc"
	NameField   = "name"
	WeightField = "weight"
)

const defWeight = uint16(100)

type attributeKey struct{}

// Info 是线缆上的地址值对象：谁、在哪、权重多少。
type Info struct {
	Name     string
	Endpoint string
	Weight   uint16
}

// Set 把地址信息挂到 resolver.Address 的属性上，供均衡器显式读取。
func Set(a resolver.Address, info *Info) resolver.Address {
	a.BalancerAttributes = a.BalancerAttributes.WithValue(attributeKey{}, info)
	return a
}

// Get 读取地址信息；取不到时返回零值而不是 nil，调用方必须处理「无权重/无名字」。
func Get(a resolver.Address) *Info {
	v := a.BalancerAttributes.Value(attributeKey{})
	if info, ok := v.(*Info); ok && info != nil {
		return info
	}
	return &Info{}
}

// Parse 解析形如 grpc://localhost:3000?weight=100&name=service1 的地址。
func Parse(s string) (*Info, error) {
	if !strings.Contains(s, "://") {
		s = Scheme + "://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("addr解析失败: %v", err)
	}
	if u.Scheme != Scheme {
		return nil, fmt.Errorf("无效的addr, 不支持的scheme: %v", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("无效的addr, endpoint为空")
	}
	if u.Path != "" {
		return nil, fmt.Errorf("无效的addr, 不应该存在path: %v", u.Path)
	}

	query := u.Query()
	name := query.Get(NameField)
	if name == "" {
		name = u.Host
	}

	weight := defWeight
	if text := query.Get(WeightField); text != "" {
		v, err := strconv.ParseUint(text, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("无效的addr, weight无法解析: %v", err)
		}
		weight = uint16(v)
	}

	return &Info{Name: name, Endpoint: u.Host, Weight: weight}, nil
}

// BindPort 从监听器取真实端口；取不到（nil 或非 TCP）时回退解析 bind 字符串。
func BindPort(ls net.Listener, bind string) int {
	if ls != nil {
		if a, ok := ls.Addr().(*net.TCPAddr); ok {
			return a.Port
		}
	}
	k := strings.LastIndex(bind, ":")
	if k == -1 {
		return 0
	}
	return cast.ToInt(bind[k+1:])
}
