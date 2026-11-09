package connreg

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"

	"github.com/zly-app/grpc/foundation/port"
)

type fakeConn struct {
	closed bool
}

func (f *fakeConn) Invoke(context.Context, string, interface{}, interface{}, ...grpc.CallOption) error {
	return nil
}
func (f *fakeConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("unsupported")
}
func (f *fakeConn) Close() error {
	f.closed = true
	return nil
}

func TestGetCreatesOnce(t *testing.T) {
	calls := 0
	made := &fakeConn{}
	c := New(func(string) (port.Conn, error) {
		calls++
		return made, nil
	})

	first, err := c.Get("svc")
	if err != nil || first != made {
		t.Fatalf("首次创建失败: %v", err)
	}
	second, err := c.Get("svc")
	if err != nil || second != made || calls != 1 {
		t.Fatalf("同名连接必须只创建一次: calls=%d err=%v", calls, err)
	}
}

func TestFailedCreateNotCached(t *testing.T) {
	calls := 0
	c := New(func(string) (port.Conn, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("transient")
		}
		return &fakeConn{}, nil
	})
	if _, err := c.Get("svc"); err == nil {
		t.Fatal("首次失败应返回错误")
	}
	if _, err := c.Get("svc"); err != nil {
		t.Fatalf("失败不应被缓存，第二次应可重试: %v", err)
	}
}

func TestCloseAllIsIdempotent(t *testing.T) {
	made := &fakeConn{}
	c := New(func(string) (port.Conn, error) { return made, nil })
	if _, err := c.Get("svc"); err != nil {
		t.Fatal(err)
	}
	c.CloseAll()
	c.CloseAll()
	if !made.closed {
		t.Fatal("CloseAll 应关闭已创建的连接")
	}
	if _, err := c.Get("svc"); err != nil {
		t.Fatal(err)
	}
}
