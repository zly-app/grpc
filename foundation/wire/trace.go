// Package wire 是底座 —— gRPC metadata 上的线缆编解码机制。
//
// 拥有：无（纯编解码）。
// 提供：OTel trace 注入/提取、CallerMeta 编解码、自定义 metadata key 编解码。
// 允许调用：无上层。禁止：在这里判定业务语义。
package wire

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	traceparentHeader = "traceparent"
	tracestateHeader  = "tracestate"
)

// TraceInjectIn 从 incoming metadata 提取对端 trace，并原样返回 mdIn 供主调信息解码。
func TraceInjectIn(ctx context.Context) (context.Context, metadata.MD) {
	mdIn, _ := metadata.FromIncomingContext(ctx)
	ctx = otel.GetTextMapPropagator().Extract(ctx, TextMapCarrier{mdIn})
	return ctx, mdIn
}

// TraceInjectOut 把当前 trace 注入 outgoing metadata，返回可直接用于发请求的 ctx 与 md 副本。
func TraceInjectOut(ctx context.Context) (context.Context, metadata.MD) {
	mdOut, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		mdOut = mdOut.Copy()
	} else {
		mdOut = metadata.New(nil)
	}
	otel.GetTextMapPropagator().Inject(ctx, TextMapCarrier{mdOut})
	return metadata.NewOutgoingContext(ctx, mdOut), mdOut
}

// TraceInjectGrpcHeader 把 trace 注入 grpc.HeaderCallOption 指定的 header。
func TraceInjectGrpcHeader(ctx context.Context, opts ...grpc.CallOption) {
	for _, o := range opts {
		if opt, ok := o.(grpc.HeaderCallOption); ok {
			if *opt.HeaderAddr == nil {
				*opt.HeaderAddr = metadata.MD{}
			}
			otel.GetTextMapPropagator().Inject(ctx, TextMapCarrier{*opt.HeaderAddr})
		}
	}
}

// TextMapCarrier 让 OTel propagator 读写 gRPC metadata。
type TextMapCarrier struct {
	MD metadata.MD
}

func (t TextMapCarrier) Get(key string) string {
	vs := t.MD[strings.ToLower(key)]
	if len(vs) > 0 {
		return vs[0]
	}
	return ""
}

func (t TextMapCarrier) Set(key string, value string) {
	if value == "" {
		return
	}
	t.MD[strings.ToLower(key)] = []string{value}
}

func (t TextMapCarrier) Keys() []string {
	keys := make([]string, 0, len(t.MD))
	for k := range t.MD {
		keys = append(keys, k)
	}
	return keys
}

var _ propagation.TextMapCarrier = (*TextMapCarrier)(nil)
