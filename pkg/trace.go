package pkg

import (
	"context"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/zly-app/zapp/pkg/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	traceparentHeader = "traceparent"
	tracestateHeader  = "tracestate"
)

func TraceInjectIn(ctx context.Context) context.Context {
	// 取出 in 元数据
	mdIn, _ := metadata.FromIncomingContext(ctx)
	tm := TextMapCarrier{mdIn}
	ctx = otel.GetTextMapPropagator().Extract(ctx, tm)
	return ctx
}
func TraceInjectOut(ctx context.Context) context.Context {
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
	return ctx
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
