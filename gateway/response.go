package gateway

import (
	"context"

	"github.com/zly-app/zapp/pkg/utils"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
)

type Response struct {
	Code    int32       `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data"`
	TraceId string      `json:"trace_id,omitempty"`
}

func ForwardResponseRewriter(ctx context.Context, response proto.Message) (any, error) {
	traceId, _ := utils.Otel.GetOTELTraceID(ctx)
	s, ok := response.(*spb.Status)
	if ok {
		return &Response{Code: s.GetCode(), Message: s.GetMessage(), TraceId: traceId}, nil
	}
	return &Response{Data: response, TraceId: traceId}, nil
}
