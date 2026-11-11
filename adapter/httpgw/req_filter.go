package httpgw

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/zly-app/zapp/filter"
	"github.com/zly-app/zapp/pkg/utils"

	"github.com/zly-app/grpc/foundation/ctxkey"
	"github.com/zly-app/grpc/model/config"
)

// reqFilter 把 HTTP 请求翻译成横切上下文，并接入 zapp 的服务端过滤器链。
// 这里没有业务规则：hashKey 判定在 model/hashkey，调用编排在 usecase.Forward。
func (g *Gateway) reqFilter(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))

		d := &ctxkey.GatewayData{
			Method:   r.Method,
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
			RawBody:  string(body),
			IP:       g.extractIP(r),
			Headers:  r.Header,
		}
		ctx := ctxkey.SaveGatewayData(r.Context(), d)
		ctx, _ = utils.Trace.GetSpanWithHeaders(ctx, d.Headers)

		callMeta := filter.GetCallerMetaByHeader(r.Header)
		ctx = filter.SaveCallerMeta(ctx, filter.CallerMeta{
			CallerInstance: callMeta.CallerInstance,
			CallerEnv:      callMeta.CallerEnv,
			CallerService:  callMeta.CallerService,
			CallerMethod:   callMeta.CallerMethod,
			CalleeService:  string(config.ServiceTypeGateway) + "/" + d.Path,
			CalleeMethod:   d.Method,
		})
		ctx, chain := filter.GetServiceFilter(ctx, string(config.ServiceTypeGateway), d.Path)
		r = r.WithContext(ctx)

		_, _ = chain.Handle(ctx, d, func(ctx context.Context, _ interface{}) (interface{}, error) {
			ctx = filter.SaveCallerMeta(ctx, filter.CallerMeta{})
			ctx, slot := ctxkey.WithResponseSlot(ctx)
			r = r.WithContext(ctx)
			h.ServeHTTP(w, r)
			return &filterResponse{Headers: w.Header(), Response: responseOf(slot)}, nil
		})
	})
}

type filterResponse struct {
	Headers  http.Header
	Response *Response
}
