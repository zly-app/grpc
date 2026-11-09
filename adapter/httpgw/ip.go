package httpgw

import (
	"net"
	"net/http"
	"strings"
)

// DefaultRemoteIPHeaders 返回默认候选 header；用函数构造，不留可变包级全局。
func DefaultRemoteIPHeaders() []string {
	return []string{"X-Original-Forwarded-For", "X-Forwarded-For", "X-Real-IP"}
}

// DefaultExtractIP 是默认的 IP 提取策略；覆盖策略由组合根的 Option 注入，
// 不再提供可随时改写的全局函数变量。
func DefaultExtractIP(req *http.Request) string {
	for _, name := range DefaultRemoteIPHeaders() {
		for _, addr := range strings.Split(req.Header.Get(name), ",") {
			if net.ParseIP(strings.TrimSpace(addr)) != nil {
				return strings.TrimSpace(addr)
			}
		}
	}
	addr := strings.TrimSpace(req.RemoteAddr)
	if addr != "" {
		if ip, _, err := net.SplitHostPort(addr); err == nil {
			return ip
		}
	}
	return addr
}
