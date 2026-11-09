package ctxkey

import "context"

// Slot 是一次 HTTP 请求与响应重写器之间共享的横切状态槽。
// 底座不认识里面的类型，只负责传播；具体 Response 由接口层定义。
type Slot struct {
	Value any
}

type responseSlotKey struct{}

// WithResponseSlot 建槽并返回同一个槽句柄；响应重写器写、请求过滤器读。
func WithResponseSlot(ctx context.Context) (context.Context, *Slot) {
	s := &Slot{}
	return context.WithValue(ctx, responseSlotKey{}, s), s
}

// ResponseSlot 取槽；取不到返回 nil，调用方必须显式处理「无上下文」分支。
func ResponseSlot(ctx context.Context) *Slot {
	s, _ := ctx.Value(responseSlotKey{}).(*Slot)
	return s
}
