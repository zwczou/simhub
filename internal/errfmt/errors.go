package errfmt

import (
	"fmt"

	"github.com/iot/simhub/apis/basepb"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Errorf 创建带 gRPC 状态码的错误
func Errorf(code codes.Code, errCode basepb.Code, errMsgs ...any) error {
	errMsg := viper.GetString(fmt.Sprintf("errors.%d", errCode))
	if errMsg != "" && len(errMsgs) > 0 {
		errMsg = fmt.Sprintf(errMsg, errMsgs...)
	}
	if errMsg == "" && len(errMsgs) == 1 {
		errMsg = cast.ToString(errMsgs[0])
	}

	st := status.New(code, errMsg)
	st, _ = st.WithDetails(NewError(errCode, errMsg))
	return st.Err()
}

// NewError 创建错误详情对象
func NewError(errCode basepb.Code, errMsg string) *basepb.Error {
	return &basepb.Error{
		Code:    uint32(errCode), // 跟php保持一致
		Message: errMsg,
	}
}

// Internal 快捷函数：内部错误
func Internal(errCode basepb.Code, errMsgs ...any) error {
	return Errorf(codes.Internal, errCode, errMsgs...)
}

// NotFound 快捷函数：未找到
func NotFound(errCode basepb.Code, errMsgs ...any) error {
	return Errorf(codes.NotFound, errCode, errMsgs...)
}

// Unauthorized 快捷函数：未授权
func Unauthorized(errCode basepb.Code, errMsgs ...any) error {
	return Errorf(codes.Unauthenticated, errCode, errMsgs...)
}

// Forbidden 快捷函数：禁止访问
func Forbidden(errCode basepb.Code, errMsgs ...any) error {
	return Errorf(codes.PermissionDenied, errCode, errMsgs...)
}

// BadRequest 快捷函数：错误请求
func BadRequest(errCode basepb.Code, errMsgs ...any) error {
	return Errorf(codes.InvalidArgument, errCode, errMsgs...)
}

// Parse 获取错误
func Parse(err error) *basepb.Error {
	if err == nil {
		return nil
	}
	st, _ := status.FromError(err)
	if st == nil {
		return nil
	}
	details := st.Details()
	if len(details) == 0 {
		return nil
	}
	return details[0].(*basepb.Error)
}
