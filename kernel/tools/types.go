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

// 数据。一个动态 Tool 来源为当前工作区准备的稳定快照。
type Snapshot struct {
	Definitions  []Definition
	Automatic    []string
	Instructions []Instruction
}

// 数据。一段只在相关 Tool 启用时加入系统提示词的来源说明。
type Instruction struct {
	Source    string
	ToolNames []string
	Text      string
}

// 数据。Agent 本轮最终启用的 Tool 与来源说明。
type Prepared struct {
	Names        []string
	Instructions []Instruction
}

// 契约。工具登记处提供的操作。
type Tools interface {
	// 固定 Tool 与动态来源管理
	Register(tool Tool) error
	RegisterProvider(provider Provider) error

	// Agent 设置与本轮准备
	Defaults() []Definition
	List() []Definition
	Prepare(ctx context.Context, workspace string, selected []string) (Prepared, error)

	// 本轮查询与调用
	Definitions(ctx context.Context, workspace string, allow []string) ([]Definition, error)
	Call(ctx context.Context, call Call) (Result, error)
}

// 契约。动态 Tool 来源按工作区提供快照并执行自己提供的 Tool。
type Provider interface {
	// 来源身份
	Name() string

	// Tool 发现
	Choices() []Definition
	Snapshot(ctx context.Context, workspace string) (Snapshot, error)

	// Tool 调用
	Call(ctx context.Context, call Call) (Result, error)
}

// 契约。一条已由 tools.New 生成参数转换的工具。
type Tool interface {
	Definition() Definition
	call(ctx context.Context, call Call, arguments json.RawMessage) (Result, error)
}
