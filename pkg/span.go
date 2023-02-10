package pkg

import (
	"context"
	"time"

	"github.com/opentracing/opentracing-go"
	open_log "github.com/opentracing/opentracing-go/log"
)

// span记录超时日志
func SpanLogDeadline(ctx context.Context, span opentracing.Span) {
	deadline, deadlineOK := ctx.Deadline()
	if !deadlineOK {
		span.LogFields(open_log.String("ctx.deadline", "false")) // 没有超时
	} else {
		d := deadline.Sub(time.Now()) // 剩余时间
		span.LogFields(open_log.String("ctx.deadline", d.String()))
	}
}
