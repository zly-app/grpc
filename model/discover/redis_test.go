package discover

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"testing"
	"time"

	rredis "github.com/redis/go-redis/v9"
	"github.com/zly-app/component/redis"

	"github.com/zly-app/grpc/model/registry"
)

// testTLSConfig 支持 rediss://：GRPC_TEST_REDIS_TLS=1 时启用 TLS。
func testTLSConfig() *tls.Config {
	if os.Getenv("GRPC_TEST_REDIS_TLS") != "1" {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: true}
}

// Redis 集成测试：环境变量门控（与 model/registry 的测试同一约定）。
//
//	GRPC_TEST_REDIS_ADDR=host:port
//	GRPC_TEST_REDIS_PASSWORD=xxx（可选）

type testCreator struct{ client redis.UniversalClient }

func (c *testCreator) GetClient(string) (redis.UniversalClient, error) { return c.client, nil }
func (c *testCreator) GetDefClient() (redis.UniversalClient, error)    { return c.client, nil }
func (c *testCreator) Close()                                         {}

func newTestDiscover(t *testing.T) (*RedisDiscover, *rredis.Client) {
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

	d, err := NewRedisDiscover(context.Background(), "test", &testCreator{client: client})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d.Close()
		_ = client.Close()
	})
	return d, client
}

// 发现侧必须纯只读：过期由 Redis 字段级 TTL 负责，读侧既不判断也不删除。
func TestRedisDiscoverIsReadOnly(t *testing.T) {
	d, client := newTestDiscover(t)
	ctx := context.Background()

	svc := fmt.Sprintf("test-discover-%d", time.Now().UnixNano())
	key := registry.RegKey(svc)
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })

	// 坏记录：不设 TTL，用来证明读侧不会把它删掉
	if err := client.HSet(ctx, key, "bad-field", "not-json").Err(); err != nil {
		t.Fatal(err)
	}
	// 合法记录：TTL 压到 1 秒，用来证明过期由服务端过滤
	if err := client.HSet(ctx, key, "10.0.0.1:1",
		`{"Name":"n@10.0.0.1:1","Endpoint":"10.0.0.1:1","Weight":100}`).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.HExpire(ctx, key, time.Second, "10.0.0.1:1").Err(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1500 * time.Millisecond)

	got, err := d.discoverOne(ctx, svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("过期字段应由 Redis 过滤, 实际返回 %d 条", len(got))
	}

	fields, err := client.HKeys(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || fields[0] != "bad-field" {
		t.Fatalf("发现侧必须只读：不应删除任何字段, 实际剩余 %v", fields)
	}
}
