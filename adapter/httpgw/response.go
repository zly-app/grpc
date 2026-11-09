package httpgw

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/zly-app/zapp/pkg/utils"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/zly-app/grpc/foundation/ctxkey"
)

// Response 是网关统一包装的 HTTP 响应格式（协议格式，不是业务规则）。
type Response struct {
	Code    int32       `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	TraceId string      `json:"trace_id,omitempty"`
}

// ForwardResponseRewriter 把 proto 响应包装成 Response，并把同一份结果写进
// 本次请求的横切状态槽：状态与通知在同一处发出，不让下游去 diff。
func ForwardResponseRewriter(ctx context.Context, response proto.Message) (any, error) {
	traceID, _ := utils.Trace.GetOTELTraceID(ctx)

	var ret *Response
	if s, ok := response.(*spb.Status); ok {
		ret = &Response{Code: s.GetCode(), Message: s.GetMessage(), TraceId: traceID}
	} else {
		ret = &Response{Data: response, TraceId: traceID}
	}
	if slot := ctxkey.ResponseSlot(ctx); slot != nil {
		slot.Value = ret
	}
	return ret, nil
}

func responseOf(slot *ctxkey.Slot) *Response {
	if slot == nil {
		return &Response{} // 无上下文时显式给空响应，不静默放行
	}
	if r, ok := slot.Value.(*Response); ok && r != nil {
		return r
	}
	return &Response{}
}

func sonicMarshal(v interface{}) (string, error) {
	return sonic.MarshalString(v)
}
