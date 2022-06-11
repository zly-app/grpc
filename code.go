package grpc

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Code = codes.Code

// 创建一个带错误码的err
func Error(c Code, msg string) error {
	return status.New(c, msg).Err()
}

// 创建一个带错误码的err
func Errorf(c Code, format string, a ...interface{}) error {
	return Error(c, fmt.Sprintf(format, a...))
}

// 获取错误状态码
func GetErrCode(err error) Code {
	if err == nil {
		return codes.OK
	}
	if se, ok := err.(interface {
		GRPCStatus() *status.Status
	}); ok {
		return se.GRPCStatus().Code()
	}
	return codes.Unknown
}
