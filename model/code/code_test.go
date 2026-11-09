package code

import (
	"context"
	"errors"
	"testing"

	"github.com/zly-app/zapp/filter"
	"google.golang.org/grpc/codes"
)

func TestErrorHelpers(t *testing.T) {
	if GetErrCode(nil) != codes.OK {
		t.Fatal("nil 错误应为 OK")
	}
	if GetErrCode(Error(codes.InvalidArgument, "bad")) != codes.InvalidArgument {
		t.Fatal("gRPC status 错误码读取失败")
	}
	if GetErrCode(Errorf(codes.Aborted, "x=%d", 1)) != codes.Aborted {
		t.Fatal("格式化错误码读取失败")
	}
	if GetErrCode(errors.New("plain")) != codes.Unknown {
		t.Fatal("普通错误应为 Unknown")
	}
}

func TestClassifyGRPCStatus(t *testing.T) {
	m := Mapper{}
	ctx := context.Background()

	c, ct, err := m.Classify(ctx, nil, Error(codes.DeadlineExceeded, "timeout"))
	if err == nil || c != int(codes.DeadlineExceeded) || ct != filter.CodeTypeTimeoutOrCancel {
		t.Fatalf("超时分类不符: code=%d type=%s err=%v", c, ct, err)
	}
	_, ct, _ = m.Classify(ctx, nil, Error(codes.Aborted, "abort"))
	if ct != filter.CodeTypeException {
		t.Fatalf("Aborted 应归 exception, 实际 %s", ct)
	}
	_, ct, _ = m.Classify(ctx, nil, Error(codes.Internal, "boom"))
	if ct != filter.CodeTypeFail {
		t.Fatalf("Internal 应归 fail, 实际 %s", ct)
	}
}

func TestClassifyPlainError(t *testing.T) {
	m := Mapper{}
	c, ct, err := m.Classify(context.Background(), nil, errors.New("boom"))
	if err == nil || c != int(codes.Internal) || ct != filter.CodeTypeFail {
		t.Fatalf("普通错误分类不符: code=%d type=%s err=%v", c, ct, err)
	}
	if statusErr, ok := m.ToStatus(ct, err); !ok || statusErr == err {
		t.Fatal("fail 应翻译成 gRPC status 错误")
	}
}

func TestClassifyNoError(t *testing.T) {
	c, ct, err := (Mapper{}).Classify(context.Background(), "rsp", nil)
	if c != 0 || ct != filter.CodeTypeSuccess || err != nil {
		t.Fatalf("无错误分类不符: code=%d type=%s err=%v", c, ct, err)
	}
}

func TestToStatusUntranslated(t *testing.T) {
	plain := errors.New("x")
	got, ok := (Mapper{}).ToStatus("unknown-type", plain)
	if ok || got != plain {
		t.Fatal("未识别的 codeType 不应替换原错误")
	}
	if got, ok := (Mapper{}).ToStatus(filter.CodeTypeFail, nil); ok || got != nil {
		t.Fatal("nil 错误不应被翻译")
	}
}
