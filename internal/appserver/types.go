// Package appserver 提供方法登记、JSON-RPC 分发和本机 WebSocket 接入，直接调用产品与公共服务。
package appserver

import internalrpc "harness/internal/appserver/internal/rpc"
import "harness/internal/appserver/internal/clientconn"

// 协议初始化与对外错误。

// 数据。本机连接初始化参数。
type InitializeParams = clientconn.InitializeParams

// 数据。初始化结果，确认双方使用的协议版本。
type InitializeResult = clientconn.InitializeResult

// 数据。稳定错误分类，不绑定 JSON-RPC 数字错误码。
type ErrorCode = internalrpc.ErrorCode

const (
	CodeUnknownMethod = internalrpc.CodeUnknownMethod
	CodeInvalidParams = internalrpc.CodeInvalidParams
	CodeNotFound      = internalrpc.CodeNotFound
	CodeConflict      = internalrpc.CodeConflict
	CodeInternal      = internalrpc.CodeInternal
)

// 数据。对调用方公开的错误与仅供进程内诊断的原因。
type Error = internalrpc.Error
