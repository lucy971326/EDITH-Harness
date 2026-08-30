// Package tools 定义工具登记处的契约。
package tools

import (
	"context"
	"encoding/json"
)

// 数据。一条工具给模型看的定义。
type Definition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// 数据。一次工具调用的上下文和模型参数。
type Call struct {
	Name      string
	Arguments json.RawMessage
	Workspace string
	Allow     []string
}

// 数据。工具调用后交还给模型的文本结果。
type Result struct {
	Content string
	IsError bool
}

// 契约。工具登记处提供的操作。
type Tools interface {
	Register(tool Tool) (unregister func(), err error)
	List() []Definition
	Definitions(allow []string) ([]Definition, error)
	Call(ctx context.Context, call Call) (Result, error)
}

// 契约。一条已由 tools.New 生成参数转换的工具。
type Tool interface {
	Definition() Definition
	call(ctx context.Context, call Call, arguments json.RawMessage) (Result, error)
}
