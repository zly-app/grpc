package route

import (
	"testing"

	"github.com/zly-app/grpc/model/config"
)

func TestHeaderFor(t *testing.T) {
	tab := New([]config.RouteConfig{
		{Path: "/hello/say", HashKeyByHeader: "x-hash-key"},
		{Path: "/no/key"},
		{HashKeyByHeader: "ignored"},
	})

	if h, ok := tab.HeaderFor("/hello/say"); !ok || h != "x-hash-key" {
		t.Fatalf("路由命中失败: h=%q ok=%v", h, ok)
	}
	if _, ok := tab.HeaderFor("/no/key"); ok {
		t.Fatal("未配 hashKey 的路由不应命中")
	}
	if _, ok := tab.HeaderFor("/unknown"); ok {
		t.Fatal("未配置 path 不应命中")
	}
}

func TestLastRuleWins(t *testing.T) {
	tab := New([]config.RouteConfig{
		{Path: "/p", HashKeyByHeader: "a"},
		{Path: "/p", HashKeyByHeader: "b"},
	})
	if h, _ := tab.HeaderFor("/p"); h != "b" {
		t.Fatalf("同 path 应以最后一条为准, 实际 %q", h)
	}
}
