package gateway

import (
	"net"
	"net/http"
	"strings"
)

var RemoteIPHeaders = []string{"X-Original-Forwarded-For", "X-Forwarded-For", "X-Real-IP"}

var RequestExtractIP = func(req *http.Request) string {
	headers := req.Header
	for _, headerName := range RemoteIPHeaders {
		ipAddresses := strings.Split(headers.Get(headerName), ",")
		for _, addr := range ipAddresses {
			if net.ParseIP(addr) != nil {
				return addr
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
