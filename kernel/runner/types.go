// Package runner 定义一轮对话运行外壳。
package runner

import "harness/kernel/session"

// 数据。一条 Runner 对外事件的种类。表面不依赖 Loop 的内部事件。
type RunEventKind string

const (
	RunStarted     RunEventKind = "run-started"
	TextDelta      RunEventKind = "text-delta"
	ReasoningDelta RunEventKind = "reasoning-delta"
	ToolStarted    RunEventKind = "tool-started"
	ToolFinished   RunEventKind = "tool-finished"
	Message        RunEventKind = "message"
	RunEnded       RunEventKind = "run-ended"
)

// 数据。一轮 Run 的结束状态。
type RunStatus string

const (
	RunSucceeded RunStatus = "success"
	RunCancelled RunStatus = "cancelled"
	RunFailed    RunStatus = "failed"
)

// 数据。工具实时事件需要的稳定事实。
type ToolEvent struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsError bool   `json:"isError"`
}

// 数据。Runner 发布给界面的一条本轮事件。
type RunEvent struct {
	SessionID string           `json:"sessionID"`
	Kind      RunEventKind     `json:"kind"`
	Text      string           `json:"text,omitempty"`
	Message   *session.Message `json:"message,omitempty"`
	Tool      *ToolEvent       `json:"tool,omitempty"`
	Status    RunStatus        `json:"status,omitempty"`
	Error     string           `json:"error,omitempty"`
}
