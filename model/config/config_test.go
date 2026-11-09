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
		t.Fatalf("默认注册器 = %q, want static", c.RegistryAddress)
	}
}

func TestServerConfigConstraints(t *testing.T) {
	c := &ServerConfig{Bind: "", HeartbeatTime: 1, PublishWeight: 0, RegistryAddress: ""}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if c.Bind != ":3000" || c.HeartbeatTime != 3 || c.PublishWeight != 100 || c.RegistryAddress != "static" {
		t.Fatalf("约束修正不符: %+v", c)
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

func TestClientConfigConstraints(t *testing.T) {
	c := &ClientConfig{BatchIncrement: 999, IdleTimeout: -1, WaitTimeout: 0}
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
