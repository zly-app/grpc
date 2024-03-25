package pkg

import (
	"context"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	jsoniter "github.com/json-iterator/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zly-app/zapp/filter"
	"github.com/zly-app/zapp/pkg/utils"
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
			traceID, _ := utils.Otel.GetOTELTraceID(ctx)
			opt.HeaderAddr.Set("trace_id", traceID)
		}
	}
}

func TraceStart(ctx context.Context, method string) context.Context {
	// 取出 in 元数据
	mdIn, _ := metadata.FromIncomingContext(ctx)
	// 取出 out 元数据
	mdOut, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		// 如果对元数据修改必须使用它的副本
		mdOut = mdOut.Copy()
	} else {
		mdOut = metadata.New(nil)
	}

	tm := TextMapCarrier{mdIn}
	ctx = otel.GetTextMapPropagator().Extract(ctx, tm)

	// 生成新的 span
	ctx, _ = utils.Otel.StartSpan(ctx, "grpc.method."+method,
		utils.OtelSpanKey("method").String(method))

	tm = TextMapCarrier{mdOut}
	otel.GetTextMapPropagator().Inject(ctx, tm)

	ctx = metadata.NewOutgoingContext(ctx, mdOut)
	return ctx
}

func TraceEnd(ctx context.Context) {
	span := utils.Otel.GetSpan(ctx)
	utils.Otel.EndSpan(span)
}

func getOtelSpanKVWithDeadline(ctx context.Context) utils.OtelSpanKV {
	deadline, deadlineOK := ctx.Deadline()
	if !deadlineOK {
		return utils.OtelSpanKey("ctx.deadline").Bool(false)
	}
	d := deadline.Sub(time.Now()) // 剩余时间
	return utils.OtelSpanKey("ctx.deadline").String(d.String())
}

func TraceReq(ctx context.Context, req interface{}) {
	span := utils.Otel.GetSpan(ctx)
	msg, _ := jsoniter.MarshalToString(req)
	utils.Otel.AddSpanEvent(span, "req",
		utils.OtelSpanKey("msg").String(msg),
		getOtelSpanKVWithDeadline(ctx),
	)
}

func TraceReply(ctx context.Context, reply interface{}, err error) {
	span := utils.Otel.GetSpan(ctx)
	if err == nil {
		msg, _ := jsoniter.MarshalToString(reply)
		utils.Otel.AddSpanEvent(span, "reply",
			utils.OtelSpanKey("msg").String(msg),
			getOtelSpanKVWithDeadline(ctx),
		)
		return
	}

	utils.Otel.MarkSpanAnError(span, true)
	utils.Otel.SetSpanAttributes(span, utils.OtelSpanKey("status.code").Int(int(status.Code(err))))

	utils.Otel.AddSpanEvent(span, "reply",
		utils.OtelSpanKey("err.detail").String(err.Error()),
		getOtelSpanKVWithDeadline(ctx),
	)
}

func TracePanic(ctx context.Context, err error) {
	span := utils.Otel.GetSpan(ctx)
	utils.Otel.SetSpanAttributes(span, utils.OtelSpanKey("panic").Bool(true))
	panicErrDetail := utils.Recover.GetRecoverErrorDetail(err)
	utils.Otel.AddSpanEvent(span, "panic",
		utils.OtelSpanKey("panic.detail").String(panicErrDetail),
		getOtelSpanKVWithDeadline(ctx),
	)
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
