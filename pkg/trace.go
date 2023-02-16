package pkg

import (
	"context"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/zly-app/zapp/pkg/utils"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	traceparentHeader = "traceparent"
	tracestateHeader  = "tracestate"
)

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

	// 提取 trace
	inTraceparent, inTracestate := mdIn.Get(traceparentHeader), mdIn.Get(tracestateHeader)
	inTraceHeader := http.Header{}
	for _, v := range inTraceparent {
		inTraceHeader.Add(traceparentHeader, v)
	}
	for _, v := range inTracestate {
		inTraceHeader.Add(tracestateHeader, v)
	}
	ctx, _ = utils.Otel.GetSpanWithHeaders(ctx, inTraceHeader)

	// 生成新的 span
	ctx, _ = utils.Otel.StartSpan(ctx, "grpc.method."+method,
		utils.OtelSpanKey("method").String(method))

	// 提取 trace 并重新写入, 由于写入时通过 http.Header 将 key 转换为大写, 儿 metadata 不支持大写 key, 所以需要转换为小写
	outTraceHeader := http.Header{}
	utils.Otel.SaveToHeaders(ctx, outTraceHeader)
	outTraceparent, outTracestate := outTraceHeader.Get(traceparentHeader), outTraceHeader.Get(tracestateHeader)
	mdOut.Set(traceparentHeader, outTraceparent)
	if outTracestate != "" {
		mdOut.Set(tracestateHeader, outTracestate)
	}

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
		utils.OtelSpanKey("detail").String(panicErrDetail),
		getOtelSpanKVWithDeadline(ctx),
	)
}
