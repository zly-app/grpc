package config

import "testing"

func TestServerConfigDefaults(t *testing.T) {
	c := NewServerConfig()
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if c.Bind != ":3000" || c.HeartbeatTime != 20 || !c.ReqDataValidate || c.PublishWeight != 100 {
		t.Fatalf("服务端默认值不符: %+v", c)
	}
	if c.RegistryAddress != "static" {
		t.Fatalf("默认注册来源 = %q, want static", c.RegistryAddress)
	}
}

func TestServerConfigConstraints(t *testing.T) {
	c := NewServerConfig()
	c.Bind = ""
	c.HeartbeatTime = 1
	c.PublishWeight = 0
	c.RegistryAddress = "  "
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if c.Bind != ":3000" || c.HeartbeatTime != 3 || c.PublishWeight != 100 || c.RegistryAddress != "static" {
		t.Fatalf("约束修正不符: %+v", c)
	}
}

// 来源字符串严格解析：未知 scheme、redis 缺组件名、服务端写直连地址都算配置错误。
func TestServerConfigRegistryAddress(t *testing.T) {
	for _, v := range []string{"static", " static ", "redis://grpc_registry"} {
		c := NewServerConfig()
		c.RegistryAddress = v
		if err := c.Check(); err != nil {
			t.Fatalf("%q 应被接受: %v", v, err)
		}
	}
	for _, v := range []string{"rediss://grpc_registry", "redis://", "consul://x", "10.0.0.1:3000", "redis:/x"} {
		c := NewServerConfig()
		c.RegistryAddress = v
		if err := c.Check(); err == nil {
			t.Fatalf("%q 应判为配置错误", v)
		}
	}
}

func TestClientConfigDefaults(t *testing.T) {
	c := NewClientConfig()
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if c.Address != "localhost:3000" || c.Balance != "weight_consistent_hash" {
		t.Fatalf("客户端默认值不符: %+v", c)
	}
	if c.MinIdle != 2 || c.MaxIdle != 4 || c.WaitTimeout != 5 || c.ConnectTimeout != 5 {
		t.Fatalf("连接池默认值不符: %+v", c)
	}
}

// 客户端地址与注册来源同构：直连列表 / static / redis://<组件名>。
func TestClientConfigAddress(t *testing.T) {
	for _, v := range []string{
		"localhost:3000",
		"10.0.0.1:3000, 10.0.0.2:3000",
		"static://10.0.0.1:3000",
		"redis://grpc_registry",
		"  ",
	} {
		c := NewClientConfig()
		c.Address = v
		if err := c.Check(); err != nil {
			t.Fatalf("%q 应被接受: %v", v, err)
		}
	}

	for _, v := range []string{
		"static",                 // 只写 static 等于没有地址
		"rediss://grpc_registry", // 未知 scheme
		"redis://",               // 缺组件名
		"10.0.0.1:3000/path",     // 地址里带 path
	} {
		c := NewClientConfig()
		c.Address = v
		if err := c.Check(); err == nil {
			t.Fatalf("%q 应判为配置错误", v)
		}
	}
}

func TestClientConfigConstraints(t *testing.T) {
	c := NewClientConfig()
	c.BatchIncrement = 999
	c.IdleTimeout = -1
	c.WaitTimeout = 0
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if c.MaxIdle != 4 || c.BatchIncrement != c.MaxIdle {
		t.Fatalf("批次增量应被最大闲置数限制: %+v", c)
	}
	if c.IdleTimeout != 0 || c.WaitTimeout != 5 {
		t.Fatalf("超时约束不符: %+v", c)
	}
}

func TestGatewayConfigDefaults(t *testing.T) {
	c := NewGatewayConfig()
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if c.Bind != ":8080" || c.CloseWait != 3 || !c.CorsAllowAll {
		t.Fatalf("网关默认值不符: %+v", c)
	}
	c2 := &GatewayConfig{CloseWait: 0}
	if err := c2.Check(); err != nil {
		t.Fatal(err)
	}
	if c2.Bind != ":8080" || c2.CloseWait != 3 {
		t.Fatalf("网关约束修正不符: %+v", c2)
	}
}

func TestGatewayHashKeyConfigNormalize(t *testing.T) {
	c := &GatewayConfig{HashKey: HashKeyConfig{
		ByHeader: "  x-key  ",
		Rules:    []HashKeyRule{{Path: "  /hello/say  ", ByHeader: " x-user-key "}},
	}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if c.HashKey.ByHeader != "x-key" || c.HashKey.Rules[0].Path != "/hello/say" || c.HashKey.Rules[0].ByHeader != "x-user-key" {
		t.Fatalf("未去除首尾空白: %+v", c.HashKey)
	}
}

func TestGatewayHashKeyConfigInvalid(t *testing.T) {
	cases := []struct {
		name string
		conf HashKeyConfig
	}{
		{"规则缺少 Path", HashKeyConfig{Rules: []HashKeyRule{{ByHeader: "x-key"}}}},
		{"Path 不以 / 开头", HashKeyConfig{Rules: []HashKeyRule{{Path: "hello", ByHeader: "x-key"}}}},
		{"Path 重复", HashKeyConfig{Rules: []HashKeyRule{{Path: "/p", ByHeader: "a"}, {Path: "/p", ByHeader: "b"}}}},
		{"默认头名非法", HashKeyConfig{ByHeader: "x key"}},
		{"规则头名非法", HashKeyConfig{Rules: []HashKeyRule{{Path: "/p", ByHeader: "x:key"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &GatewayConfig{HashKey: tc.conf}
			if err := c.Check(); err == nil {
				t.Fatalf("应判为配置错误: %+v", tc.conf)
			}
		})
	}
}
