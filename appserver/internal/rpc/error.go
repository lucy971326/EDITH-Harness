// Package rpc 负责类型化方法登记、契约校验与必要的 Session 写请求排序。
package rpc

// 数据。稳定错误分类，不绑定 JSON-RPC 数字错误码。
type ErrorCode string

const (
	CodeUnknownMethod ErrorCode = "unknown_method"
	CodeInvalidParams ErrorCode = "invalid_params"
	CodeNotFound      ErrorCode = "not_found"
	CodeConflict      ErrorCode = "conflict"
	CodeInternal      ErrorCode = "internal"
)

// 数据。对调用方公开的错误与仅供进程内诊断的原因。
type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Cause   error     `json:"-"`
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }
func (e *Error) Unwrap() error { return e.Cause }
