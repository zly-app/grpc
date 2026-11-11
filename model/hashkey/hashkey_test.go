package hashkey

import (
	"testing"

	"github.com/zly-app/grpc/model/config"
)

func TestHeaderFor(t *testing.T) {
	tab := New(config.HashKeyConfig{
		ByHeader: "x-default-key",
		Rules: []config.HashKeyRule{
			{Path: "/hello/say", ByHeader: "x-user-key"},
			{Path: "/hello/off"}, // 规则命中但头名为空：显式不参与 hash
		},
	})

	if h, ok := tab.HeaderFor("/hello/say"); !ok || h != "x-user-key" {
		t.Fatalf("规则覆盖失败: h=%q ok=%v", h, ok)
	}
	if h, ok := tab.HeaderFor("/hello/other"); !ok || h != "x-default-key" {
		t.Fatalf("默认来源未生效: h=%q ok=%v", h, ok)
	}
	if h, ok := tab.HeaderFor("/hello/off"); ok || h != "" {
		t.Fatalf("显式关闭的 path 不应回落默认: h=%q ok=%v", h, ok)
	}
}

func TestNoConfig(t *testing.T) {
	tab := New(config.HashKeyConfig{})
	if h, ok := tab.HeaderFor("/any"); ok || h != "" {
		t.Fatalf("未配置时不应命中: h=%q ok=%v", h, ok)
	}
}

func TestDefaultAppliesToEmptyPath(t *testing.T) {
	tab := New(config.HashKeyConfig{ByHeader: "x-key"})
	if h, ok := tab.HeaderFor(""); !ok || h != "x-key" {
		t.Fatalf("默认来源应覆盖取不到 path 的请求: h=%q ok=%v", h, ok)
	}
}

func TestEmptyPathRuleIgnored(t *testing.T) {
	tab := New(config.HashKeyConfig{
		Rules: []config.HashKeyRule{{Path: "", ByHeader: "x-key"}},
	})
	if _, ok := tab.HeaderFor(""); ok {
		t.Fatal("空 Path 规则应被忽略，不能兜住所有请求")
	}
}
