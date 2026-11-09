package resolver

import (
	"sync"
	"testing"

	"google.golang.org/grpc/resolver"
)

// recordingCC 只实现 UpdateState，其余方法由嵌入接口满足。
type recordingCC struct {
	resolver.ClientConn
	mx    sync.Mutex
	calls int
}

func (r *recordingCC) UpdateState(resolver.State) error {
	r.mx.Lock()
	r.calls++
	r.mx.Unlock()
	return nil
}

func (r *recordingCC) count() int {
	r.mx.Lock()
	defer r.mx.Unlock()
	return r.calls
}

// 会停测试：广播是单向、有限、不回灌的；每次 UpdateState 恰好推一轮，
// 关闭的 ClientConn 不再收到更新。证明「地址广播环」有终止保障。
func TestUpdateStateTerminates(t *testing.T) {
	b := NewBuilderWithScheme("static")
	b.InitialState(resolver.State{Addresses: []resolver.Address{{Addr: "127.0.0.1:1"}}})

	cc := &recordingCC{}
	r, err := b.Build(resolver.Target{}, cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cc.count() != 1 {
		t.Fatalf("Build 应推一次初始状态, 实际 %d", cc.count())
	}

	const rounds = 100
	for i := 0; i < rounds; i++ {
		b.UpdateState(resolver.State{Addresses: []resolver.Address{{Addr: "127.0.0.1:2"}}})
	}
	if got := cc.count(); got != 1+rounds {
		t.Fatalf("广播次数 = %d, want %d（不得回灌或递归放大）", got, 1+rounds)
	}

	r.Close()
	b.UpdateState(resolver.State{})
	if got := cc.count(); got != 1+rounds {
		t.Fatalf("ClientConn 关闭后仍收到广播: %d", got)
	}
}

func TestScheme(t *testing.T) {
	b := NewBuilderWithScheme("static")
	if b.Scheme() != "static" {
		t.Fatalf("scheme = %q", b.Scheme())
	}
}
