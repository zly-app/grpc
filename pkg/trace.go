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

func TraceStart(ctx context.Context, method string) context.Context {
	// 取出元数据
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		// 如果对元数据修改必须使用它的副本
		md = md.Copy()
	} else {
		md = metadata.New(nil)
	}

	ctx, _ = utils.Otel.GetSpanWithHeaders(ctx, http.Header(md))
	ctx, _ = utils.Otel.StartSpan(ctx, "grpc.method."+method,
		utils.OtelSpanKey("method").String(method))
	utils.Otel.SaveToHeaders(ctx, http.Header(md))
	ctx = metadata.NewIncomingContext(ctx, md)
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
	utils.Otel.AddSpanEvent(span, "send",
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
