package pkg

import (
	"context"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/zly-app/zapp/filter"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	traceparentHeader = "traceparent"
	tracestateHeader  = "tracestate"
)

// 取出 mdIn, 返回的 ctx 会带上 trace (如果上游传入的trace)
func TraceInjectIn(ctx context.Context) (context.Context, metadata.MD) {
	// 取出 in 元数据
	mdIn, _ := metadata.FromIncomingContext(ctx)
	tm := TextMapCarrier{mdIn}
	ctx = otel.GetTextMapPropagator().Extract(ctx, tm)
	return ctx, mdIn
}

// 返回的 ctx 如果用于 client 会把当前 trace 带上
func TraceInjectOut(ctx context.Context) (context.Context, metadata.MD) {
	// 取出 out 元数据
	mdOut, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		// 如果对元数据修改必须使用它的副本
		mdOut = mdOut.Copy()
	} else {
		mdOut = metadata.New(nil)
	}

	tm := TextMapCarrier{mdOut}
	otel.GetTextMapPropagator().Inject(ctx, tm)
	ctx = metadata.NewOutgoingContext(ctx, mdOut)
	return ctx, mdOut
}

// trace注入到grpcHeader中
func TraceInjectGrpcHeader(ctx context.Context, opts ...grpc.CallOption) {
	for _, o := range opts {
		switch opt := o.(type) {
		case grpc.HeaderCallOption:
			if *opt.HeaderAddr == nil {
				*opt.HeaderAddr = metadata.MD{}
			}
			tm := TextMapCarrier{*opt.HeaderAddr}
			otel.GetTextMapPropagator().Inject(ctx, tm)
		}
	}
}

type TextMapCarrier struct {
	md metadata.MD
}

func (t TextMapCarrier) Get(key string) string {
	key = strings.ToLower(key)
	vs := t.md[key]
	if len(vs) > 0 {
		return vs[0]
	}
	return ""
}

func (t TextMapCarrier) Set(key string, value string) {
	if value == "" {
		return
	}
	key = strings.ToLower(key)
	t.md[key] = []string{value}
}

func (t TextMapCarrier) Keys() []string {
	keys := make([]string, 0, len(t.md))
	for k := range t.md {
		keys = append(keys, k)
	}
	return keys
}

var _ propagation.TextMapCarrier = (*TextMapCarrier)(nil)

const mdCallerMetaKey = "caller_meta"

func ExtractCallerMetaFromMD(md metadata.MD) (filter.CallerMeta, bool) {
	ss := md.Get(mdCallerMetaKey)
	if len(ss) == 0 {
		return filter.CallerMeta{}, false
	}
	meta := filter.CallerMeta{}
	err := sonic.UnmarshalString(ss[0], &meta)
	return meta, err == nil
}

func InjectCallerMetaToMD(ctx context.Context, mdCopy metadata.MD, callerMeta filter.CallerMeta) context.Context {
	metaText, err := sonic.MarshalString(callerMeta)
	if err != nil {
		return ctx
	}
	mdCopy.Set(mdCallerMetaKey, metaText)
	ctx = metadata.NewOutgoingContext(ctx, mdCopy)
	return ctx
}
