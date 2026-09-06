// Package loops 定义运行范式登记处的契约。
package loops

import (
	"context"

	"harness/kernel/llm"
	"harness/kernel/session"
)

// 数据。一种可选运行范式的说明。
type Definition struct {
	Kind        string
	Description string
}

// 数据。Loop 产出的一条本轮事件。
type Event struct {
	Kind     EventKind
	StepSeq  uint64
	BlockSeq uint64
	Text     string
	Message  *session.Message
	Tool     *ToolEvent
	Usage    *Usage
}

// 数据。与工具调用有关的事件内容。
type ToolEvent struct {
	ID      string
	Name    string
	IsError bool
}

// 数据。最近一次模型调用的输入占用。不进账本。
type Usage struct {
	InputTokens     int
	CacheReadTokens int
	ContextWindow   int
}

// 数据。一条 Loop 事件的种类。
type EventKind string

const (
	EventTextDelta      EventKind = "text-delta"
	EventReasoningDelta EventKind = "reasoning-delta"
	EventToolStarted    EventKind = "tool-started"
	EventToolFinished   EventKind = "tool-finished"
	EventMessage        EventKind = "message"
	EventUsage          EventKind = "usage"
)

// 数据。一次检查点之后 Loop 是否还必然继续。
type CheckpointPhase uint8

const (
	CheckpointContinue CheckpointPhase = iota
	CheckpointFinal
)

// 数据。Runner 已准备好、交给 Loop 的本轮输入。
type Invocation struct {
	// 本轮由 Runner 提供的稳定身份。
	SessionID    string
	RunID        string
	History      []session.Message
	SystemPrompt string
	LLMConfig    llm.RunConfig
	ToolNames    []string
	Workspace    string
	// Steer 到达时唤醒等待者；调用时取得当前代次，消息仍由 Checkpoint 消费。
	InputSignal func() <-chan struct{}
	Emit        func(context.Context, Event) error
	Checkpoint  func(context.Context, CheckpointPhase) ([]session.Message, error)
}

// 契约。一种已编译的运行范式。
type Loop interface {
	Definition() Definition
	Run(ctx context.Context, invocation Invocation) error
}

// 契约。Loop 登记处提供的操作。
type Loops interface {
	Register(loop Loop) error
	Get(kind string) (Loop, error)
	Definitions() []Definition
}
