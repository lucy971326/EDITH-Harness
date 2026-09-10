// Package appserver 提供方法登记、JSON-RPC 分发和本机 WebSocket 接入，不依赖具体产品。
package appserver

// 协议初始化与对外错误。

// 数据。本机连接初始化参数。
type InitializeParams struct {
	ProtocolVersion int `json:"protocolVersion"`
}

// 数据。初始化结果，确认双方使用的协议版本。
type InitializeResult struct {
	ProtocolVersion int `json:"protocolVersion"`
}

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
