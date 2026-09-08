// Package appserver 定义产品接口契约和进程内分发，不依赖具体产品或传输。
package appserver

import (
	"context"
	"encoding/json"
)

// 契约。方法的名称、说明和输入输出 Go 类型；同一声明用于登记和生成。
type Method[Input, Output any] struct {
	Name        string
	Description string
}

// 契约。处理函数只接收已校验的输入，返回仍需校验的输出。
type Handler[Input, Output any] func(context.Context, Input) (Output, error)

// 数据。可导出的接口目录条目，Schema 由 Go 类型生成。
type Definition struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema"`
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
