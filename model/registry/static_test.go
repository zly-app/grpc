package registry

import (
	"context"
	"sync"
	"testing"

	gresolver "google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/foundation/addr"
)

type recordingCC struct {
	gresolver.ClientConn
	mx     sync.Mutex
	states []gresolver.State
}

func (r *recordingCC) UpdateState(s gresolver.State) error {
	r.mx.Lock()
	r.states = append(r.states, s)
	r.mx.Unlock()
	return nil
}

func (r *recordingCC) count() int {
	r.mx.Lock()
	defer r.mx.Unlock()
	return len(r.states)
}

func (r *recordingCC) last() gresolver.State {
	r.mx.Lock()
	defer r.mx.Unlock()
	return r.states[len(r.states)-1]
}

// 唯一施放入口：写状态与广播通知在同一次 Register 调用里完成，
// 已连接的 client 无需等下次拨号就能看到新地址。
func TestRegisterBroadcastsImmediately(t *testing.T) {
	s := NewStaticTable()
	ctx := context.Background()

	if err := s.Register(ctx, "svc", &addr.Info{Endpoint: "10.0.0.1:1", Weight: 100}); err != nil {
		t.Fatal(err)
	}
	b, err := s.Builder(ctx, "svc")
	if err != nil {
		t.Fatal(err)
	}
	cc := &recordingCC{}
	r, err := b.Build(gresolver.Target{}, cc, gresolver.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cc.count() != 1 {
		t.Fatalf("Build 应推一次初始状态, 实际 %d", cc.count())
	}

	if err := s.Register(ctx, "svc", &addr.Info{Endpoint: "10.0.0.1:2", Weight: 50}); err != nil {
		t.Fatal(err)
	}
	if cc.count() != 2 {
		t.Fatalf("注册后必须同点广播, 实际广播次数 %d", cc.count())
	}
	if got := len(cc.last().Addresses); got != 2 {
		t.Fatalf("广播状态应含 2 个地址, 实际 %d", got)
	}
	if w := addr.Get(cc.last().Addresses[1]).Weight; w != 50 {
		t.Fatalf("广播状态权重 = %d, want 50", w)
	}

	r.Close()
	if err := s.Register(ctx, "svc", &addr.Info{Endpoint: "10.0.0.1:3"}); err != nil {
		t.Fatal(err)
	}
	if cc.count() != 2 {
		t.Fatal("已关闭的 ClientConn 不应再收到广播（终止保障）")
	}
}

func TestUnregisterBroadcastsEmpty(t *testing.T) {
	s := NewStaticTable()
	ctx := context.Background()
	_ = s.Register(ctx, "svc", &addr.Info{Endpoint: "10.0.0.1:1"})
	b, _ := s.Builder(ctx, "svc")
	cc := &recordingCC{}
	_, _ = b.Build(gresolver.Target{}, cc, gresolver.BuildOptions{})

	s.Unregister(ctx, "svc")
	if cc.count() != 2 || len(cc.last().Addresses) != 0 {
		t.Fatalf("注销应广播空地址集, count=%d addrs=%d", cc.count(), len(cc.last().Addresses))
	}
	if _, err := s.Builder(ctx, "svc"); err == nil {
		t.Fatal("地址清空后 Builder 必须显式报错")
	}
}

func TestRegisterRejectsEmptyAddress(t *testing.T) {
	s := NewStaticTable()
	if err := s.Register(context.Background(), "svc", nil); err == nil {
		t.Fatal("空地址注册应显式报错")
	}
}

func TestParseAddress(t *testing.T) {
	cases := []struct {
		in        string
		typ, addr string
	}{
		{"", TypeStatic, ""},
		{"static", TypeStatic, ""},
		{"localhost:3000", TypeStatic, "localhost:3000"},
		{"redis://myredis", TypeRedis, "myredis"},
		{"REDIS://myredis", TypeRedis, "myredis"},
	}
	for _, c := range cases {
		got := ParseAddress(c.in)
		if got.Type != c.typ || got.Addr != c.addr {
			t.Fatalf("ParseAddress(%q) = %+v, want {%s %s}", c.in, got, c.typ, c.addr)
		}
	}
}

func TestProtocolKeysAreShared(t *testing.T) {
	if SeqKey("svc") != KeySeqIncr+"svc" || RegKey("svc") != KeyServerReg+"svc" ||
		SignalKey("svc") != KeyRegSignal+"svc" || RegField(7) != "7" {
		t.Fatal("注册/发现共享的 key 协议不一致")
	}
}
