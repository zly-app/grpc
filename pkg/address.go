package pkg

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cast"
	"google.golang.org/grpc/resolver"
)

type attributeKey struct{}

const (
	Scheme      = "grpc"
	NameField   = "name"
	WeightField = "weight"
)

const defWeight = uint16(100)

type AddrInfo struct {
	Name     string // 名称
	Endpoint string // 端点
	Weight   uint16 // 权重
}

// 设置信息
func SetAddrInfo(addr resolver.Address, addrInfo *AddrInfo) resolver.Address {
	addr.BalancerAttributes = addr.BalancerAttributes.WithValue(attributeKey{}, addrInfo)
	return addr
}

// 获取信息
func GetAddrInfo(addr resolver.Address) *AddrInfo {
	v := addr.BalancerAttributes.Value(attributeKey{})
	ai, ok := v.(*AddrInfo)
	if !ok {
		ai = &AddrInfo{}
	}
	return ai
}

// 解析addr, 示例: grpc://localhost:3000?weight=100&name=service1
func ParseAddr(addr string) (*AddrInfo, error) {
	if !strings.Contains(addr, "://") {
		addr = Scheme + "://" + addr
	}

	u, err := url.Parse(addr)
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

	endpoint := u.Host
	query := u.Query()

	name := query.Get(NameField)
	if name == "" {
		name = endpoint
	}

	weight := defWeight
	weightText := query.Get(WeightField)
	if weightText != "" {
		v, err := strconv.ParseUint(weightText, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("无效的addr, weight无法解析: %v", err)
		}
		weight = uint16(v)
	}

	out := &AddrInfo{
		Name:     name,
		Endpoint: endpoint,
		Weight:   weight,
	}
	return out, err
}

// 解析绑定端口
func ParseBindPort(ls net.Listener, bind string) int {
	addr, ok := ls.Addr().(*net.TCPAddr)
	if ok {
		return addr.Port
	}

	k := strings.LastIndex(bind, ":")
	if k == -1 {
		return 0
	}
	return cast.ToInt(bind[k+1:])
}
