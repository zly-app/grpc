// Package code 是模型层 —— 错误码规则的唯一所有者。
//
// 拥有：gRPC code ↔ zapp codeType 的映射规则、默认错误分类规则。
// 提供：Mapper.Classify / ToStatus / Install，以及 Error / Errorf / GetErrCode。
// 允许调用：zapp/filter（读取契约类型）、zapp/pkg/utils（recover 判定）。
// 禁止：在各拦截器里各写一份 code 映射；禁止在包 init 里改依赖方全局。
package code

import (
	"context"
	"fmt"
	"sync"

	"github.com/zly-app/zapp/filter"
	"github.com/zly-app/zapp/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Code 是 gRPC 状态码。
type Code = codes.Code

// Error 创建一个带错误码的 error。
func Error(c Code, msg string) error { return status.New(c, msg).Err() }

// Errorf 创建一个带错误码的格式化 error。
func Errorf(c Code, format string, a ...interface{}) error {
	return Error(c, fmt.Sprintf(format, a...))
}

// GetErrCode 获取错误状态码；非 gRPC 错误按 Unknown 处理。
func GetErrCode(err error) Code {
	if err == nil {
		return codes.OK
	}
	if se, ok := err.(interface{ GRPCStatus() *status.Status }); ok {
		return se.GRPCStatus().Code()
	}
	return codes.Unknown
}

// Mapper 拥有错误分类与状态码翻译规则，不持有状态。
type Mapper struct{}

// Classify 把一次调用的响应/错误归类成 (gRPC 码, codeType, 原错误)。
// gRPC status 错误优先；其余按 zapp 默认规则分类后再翻译成 gRPC 码。
func (Mapper) Classify(ctx context.Context, rsp interface{}, err error) (int, string, error) {
	if err != nil {
		if se, ok := err.(interface{ GRPCStatus() *status.Status }); ok {
			c := se.GRPCStatus().Code()
			switch c {
			case codes.OK:
				return 0, filter.CodeTypeSuccess, nil
			case codes.DeadlineExceeded:
				return int(c), filter.CodeTypeTimeoutOrCancel, err
			case codes.Aborted:
				return int(c), filter.CodeTypeException, err
			}
			return int(c), filter.CodeTypeFail, err
		}
	}

	code, codeType, replaceErr := classifyDefault(ctx, err)
	switch codeType {
	case filter.CodeTypeTimeoutOrCancel:
		return int(codes.DeadlineExceeded), codeType, replaceErr
	case filter.CodeTypeFail:
		return int(codes.Internal), codeType, replaceErr
	case filter.CodeTypeException:
		return int(codes.Aborted), codeType, replaceErr
	}
	return code, codeType, replaceErr
}

// ToStatus 把非 gRPC 错误按 codeType 翻译成 gRPC status 错误；
// 第二个返回值说明是否发生了翻译（调用方据此决定要不要替换响应）。
func (Mapper) ToStatus(codeType string, err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	switch codeType {
	case filter.CodeTypeTimeoutOrCancel:
		return status.New(codes.DeadlineExceeded, err.Error()).Err(), true
	case filter.CodeTypeFail:
		return status.New(codes.Internal, err.Error()).Err(), true
	case filter.CodeTypeException:
		return status.New(codes.Aborted, err.Error()).Err(), true
	}
	return err, false
}

// classifyDefault 内联 zapp filter 的默认分类规则，避免 Classify 与
// filter.DefaultGetErrCodeFunc 互相调用造成递归。
func classifyDefault(ctx context.Context, err error) (int, string, error) {
	if err == nil {
		return 0, filter.CodeTypeSuccess, nil
	}
	if err == context.DeadlineExceeded {
		return -1, filter.CodeTypeTimeoutOrCancel, err
	}
	meta := filter.GetCallMeta(ctx)
	if meta.HasPanic() {
		return -2, filter.CodeTypeException, err
	}
	if utils.Recover.IsRecoverError(err) {
		return -2, filter.CodeTypeException, err
	}
	return -3, filter.CodeTypeFail, err
}

// installOnce 只是幂等守卫，不持有业务状态：
// 与 zapp filter 的接壤是进程级全局，重复装配会层层包装同一个函数。
var installOnce sync.Once

// Install 显式把 gRPC 错误码规则装到 zapp filter 链上。
// 只能由组合根调用，替代旧版根包 init() 的隐式覆写。
func Install() {
	installOnce.Do(func() {
		filter.DefaultGetErrCodeFunc = (Mapper{}).Classify
	})
}
