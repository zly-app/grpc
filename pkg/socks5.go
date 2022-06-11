package pkg

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"
)

type ISocks5Proxy interface {
	Dial(network, addr string) (c net.Conn, err error)
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type Socks5Proxy struct {
	dial        func(network, addr string) (c net.Conn, err error)
	dialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

func (s *Socks5Proxy) Dial(network, addr string) (c net.Conn, err error) {
	return s.dial(network, addr)
}

func (s *Socks5Proxy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return s.dialContext(ctx, network, address)
}

/*创建一个socks5代理
  address 代理地址. 支持socks5, socks5h. 示例: socks5://127.0.0.1:1080
  user 用户名
  passwd 密码
*/
func NewSocks5Proxy(address, user, passwd string) (ISocks5Proxy, error) {
	// 解析地址
	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("address无法解析: %v", err)
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if user != "" || passwd != "" {
			auth = &proxy.Auth{User: user, Password: passwd}
		}

		dialer, err := proxy.SOCKS5("tcp", u.Host, auth, nil)
		if err != nil {
			return nil, fmt.Errorf("dialer生成失败: %v", err)
		}

		var dialCtx func(ctx context.Context, network, address string) (net.Conn, error)
		if d, ok := dialer.(proxy.ContextDialer); ok {
			dialCtx = d.DialContext
		} else {
			dialCtx = func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialer.Dial(network, address)
			}
		}

		sp := &Socks5Proxy{
			dial:        dialer.Dial,
			dialContext: dialCtx,
		}
		return sp, nil
	}
	return nil, fmt.Errorf("address的scheme不支持: %s")
}
