package gateway

import (
	"io"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	spb "google.golang.org/genproto/googleapis/rpc/status"
)

type Marshaler struct {
	runtime.Marshaler
}
type Response struct {
	Code    int32       `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (m *Marshaler) Marshal(v interface{}) ([]byte, error) {
	rsp := WrapResponse(v)
	return m.Marshaler.Marshal(rsp)
}
func (m *Marshaler) NewEncoder(w io.Writer) runtime.Encoder {
	e := m.Marshaler.NewEncoder(w)
	return &Encoder{Encoder: e}
}

type Encoder struct {
	runtime.Encoder
}

func (e *Encoder) Encode(v interface{}) error {
	rsp := WrapResponse(v)
	return e.Encoder.Encode(rsp)
}

func WrapResponse(v interface{}) interface{} {
	if s, ok := v.(*spb.Status); ok {
		return &Response{
			Code:    s.Code,
			Message: s.Message,
		}
	}
	rsp := &Response{Data: v}
	return rsp
}
