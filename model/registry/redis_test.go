package registry

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	rredis "github.com/redis/go-redis/v9"
	"github.com/zly-app/component/redis"

	"github.com/zly-app/grpc/foundation/addr"
)

// testTLSConfig 支持 rediss://：GRPC_TEST_REDIS_TLS=1 时启用 TLS。
// 内网用 mkcert 开发 CA，与 Apollo 下发的 InsecureSkipVerify 策略保持一致。
func testTLSConfig() *tls.Config {
	if os.Getenv("GRPC_TEST_REDIS_TLS") != "1" {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: true}
}

// Redis 集成测试：需要真实 Redis 8.x（HSETEX/HEXPIRE 为 8.0 能力），
// miniredis 不支持字段级 TTL，因此用环境变量门控，默认跳过。
//
//	GRPC_TEST_REDIS_ADDR=host:port
//	GRPC_TEST_REDIS_PASSWORD=xxx（可选）

type testCreator struct{ client redis.UniversalClient }

func (c *testCreator) GetClient(string) (redis.UniversalClient, error) { return c.client, nil }
func (c *testCreator) GetDefClient() (redis.UniversalClient, error)    { return c.client, nil }
func (c *testCreator) Close()                                         {}

func newTestRegistry(t *testing.T) (*RedisRegistry, *rredis.Client) {
	t.Helper()

	addrStr := os.Getenv("GRPC_TEST_REDIS_ADDR")
	if addrStr == "" {
		t.Skip("未设置 GRPC_TEST_REDIS_ADDR，跳过 Redis 集成测试")
	}
	client := rredis.NewClient(&rredis.Options{
		Addr:      addrStr,
		Password:  os.Getenv("GRPC_TEST_REDIS_PASSWORD"),
		TLSConfig: testTLSConfig(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("连接测试 redis 失败: %v", err)
	}

	r, err := NewRedisRegistry(context.Background(), "test", &testCreator{client: client})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Close()
		_ = client.Close()
	})
	return r, client
}

func uniqueServerName(tag string) string {
	return fmt.Sprintf("test-%s-%d", tag, time.Now().UnixNano())
}

func cleanupKey(t *testing.T, client *rredis.Client, key string) {
	t.Helper()
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })
}

// 注册一条命令落地：值与字段级 TTL 由 HSETEX 同时写入，客户端不需要任何删除动作。
func TestRedisRegisterWritesFieldWithTTL(t *testing.T) {
	r, client := newTestRegistry(t)
	ctx := context.Background()

	svc := uniqueServerName("reg")
	key := RegKey(svc)
	cleanupKey(t, client, key)

	if err := r.Register(ctx, svc, &addr.Info{Name: "hello.pub", Endpoint: "10.0.0.1:3000", Weight: 200}); err != nil {
		t.Fatal(err)
	}

	data, err := client.HGetAll(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 {
		t.Fatalf("注册后应只有一条记录, 实际 %v", data)
	}
	text, ok := data["10.0.0.1:3000"]
	if !ok {
		t.Fatalf("field 必须等于 endpoint, 实际 %v", data)
	}
	reg := RegServer{}
	if err := sonic.UnmarshalString(text, &reg); err != nil {
		t.Fatal(err)
	}
	if reg.Endpoint != "10.0.0.1:3000" || reg.Weight != 200 {
		t.Fatalf("记录内容不符: %+v", reg)
	}
	if reg.Name != "hello.pub@10.0.0.1:3000" {
		t.Fatalf("Name 必须是「前缀@endpoint」形式: %q", reg.Name)
	}

	ttl, err := client.HTTL(ctx, key, RegField("10.0.0.1:3000")).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(ttl) != 1 || ttl[0] <= RegistryExpire/2 {
		t.Fatalf("注册必须带字段级 TTL, ttl=%v", ttl)
	}
}

// 同 endpoint 重复注册是幂等覆盖：不产生第二条记录，TTL 被续满。
func TestRedisRegisterIsIdempotentPerEndpoint(t *testing.T) {
	r, client := newTestRegistry(t)
	ctx := context.Background()

	svc := uniqueServerName("idem")
	key := RegKey(svc)
	field := RegField("10.0.0.9:3000")
	cleanupKey(t, client, key)

	for i := 0; i < 2; i++ {
		if err := r.Register(ctx, svc, &addr.Info{Endpoint: "10.0.0.9:3000", Weight: uint16(100 + i)}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := client.HLen(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("同 endpoint 重复注册不应新增记录, 实际 %d 条", n)
	}

	reg := RegServer{}
	text, _ := client.HGet(ctx, key, field).Result()
	_ = sonic.UnmarshalString(text, &reg)
	if reg.Weight != 101 {
		t.Fatalf("重复注册应覆盖为最新值, 实际 weight=%d", reg.Weight)
	}
}

// 续租就是重设同一字段的 TTL：不依赖读侧判断，也没有「记录已被删需要重建」的分支。
func TestRedisLeaseRefreshesTTL(t *testing.T) {
	r, client := newTestRegistry(t)
	ctx := context.Background()

	svc := uniqueServerName("lease")
	key := RegKey(svc)
	field := RegField("10.0.0.2:3000")
	cleanupKey(t, client, key)

	if err := r.Register(ctx, svc, &addr.Info{Endpoint: "10.0.0.2:3000"}); err != nil {
		t.Fatal(err)
	}
	if err := client.HExpire(ctx, key, time.Second, field).Err(); err != nil {
		t.Fatal(err)
	}
	if ttl, _ := client.HTTL(ctx, key, field).Result(); len(ttl) != 1 || ttl[0] > 2 {
		t.Fatalf("前置条件不符: 期望 TTL 已被压到 1s, 实际 %v", ttl)
	}

	if err := r.touch(ctx, svc, &RegServer{Name: field, Endpoint: field, Weight: 100}); err != nil {
		t.Fatal(err)
	}
	ttl, err := client.HTTL(ctx, key, field).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(ttl) != 1 || ttl[0] < RegistryExpire-1 {
		t.Fatalf("续租后 TTL 应回到接近 %ds, 实际 %v", RegistryExpire, ttl)
	}
}

// 崩溃场景：没有任何客户端删除动作，Redis 自己让字段过期。
func TestRedisFieldExpiresWithoutDelete(t *testing.T) {
	r, client := newTestRegistry(t)
	ctx := context.Background()

	svc := uniqueServerName("expire")
	key := RegKey(svc)
	field := RegField("10.0.0.3:3000")
	cleanupKey(t, client, key)

	if err := r.Register(ctx, svc, &addr.Info{Endpoint: "10.0.0.3:3000"}); err != nil {
		t.Fatal(err)
	}
	if err := client.HExpire(ctx, key, time.Second, field).Err(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		data, err := client.HGetAll(ctx, key).Result()
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("字段级 TTL 到期后应由 Redis 自动移除, 仍在: %v", data)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// 注销：删字段 + 发取消信号；字段删除是加速下线，TTL 才是兜底。
func TestRedisUnregisterRemovesFieldAndPublishes(t *testing.T) {
	r, client := newTestRegistry(t)
	ctx := context.Background()

	svc := uniqueServerName("unreg")
	key := RegKey(svc)
	cleanupKey(t, client, key)

	sub := client.Subscribe(ctx, SignalKey(svc))
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("订阅信号失败: %v", err)
	}

	if err := r.Register(ctx, svc, &addr.Info{Endpoint: "10.0.0.4:3000"}); err != nil {
		t.Fatal(err)
	}
	r.Unregister(ctx, svc)

	n, err := client.HLen(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("注销后字段应被删除, 实际仍有 %d 条", n)
	}

	timeout := time.After(3 * time.Second)
	for {
		select {
		case msg, ok := <-sub.Channel():
			if !ok {
				t.Fatal("信号通道已关闭")
			}
			sig := RegSignal{}
			if err := sonic.UnmarshalString(msg.Payload, &sig); err != nil || sig.Reg == nil {
				continue
			}
			if sig.IsUnReg && sig.Reg.Endpoint == "10.0.0.4:3000" {
				return
			}
		case <-timeout:
			t.Fatal("未收到取消注册信号")
		}
	}
}
