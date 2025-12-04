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
	Data    interface{} `json:"data,omitempty"`
	TraceId string      `json:"trace_id,omitempty"`
}

func ForwardResponseRewriter(ctx context.Context, response proto.Message) (any, error) {
	var ret *Response

	traceId, _ := utils.Trace.GetOTELTraceID(ctx)
	s, ok := response.(*spb.Status)
	if ok {
		ret = &Response{Code: s.GetCode(), Message: s.GetMessage(), TraceId: traceId}
	} else {
		ret = &Response{Data: response, TraceId: traceId}
	}
	saveResponse(ctx, ret)
	return ret, nil
}

type responseStorageFlag struct{}

func initResponseStorage(ctx context.Context) context.Context {
	return context.WithValue(ctx, responseStorageFlag{}, &Response{})
}

func saveResponse(ctx context.Context, res *Response) {
	v := ctx.Value(responseStorageFlag{})
	if storage, ok := v.(*Response); ok {
		*storage = *res
	}
}

func getResponse(ctx context.Context) *Response {
	v := ctx.Value(responseStorageFlag{})
	if storage, ok := v.(*Response); ok {
		return storage
	}
	return &Response{}
}
