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

// 数据。一场尚未结束 Run 的可恢复运行事实。
type RunState struct {
	RunID         string `json:"runID"`
	AfterEntrySeq uint64 `json:"afterEntrySeq"`
}

// 数据。工具实时事件需要的稳定事实。
type ToolEvent struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsError bool   `json:"isError"`
}

// 数据。Runner 发布给界面的一条本轮事件。
type RunEvent struct {
	SessionID     string         `json:"sessionID"`
	RunID         string         `json:"runID"`
	Kind          RunEventKind   `json:"kind"`
	AfterEntrySeq uint64         `json:"afterEntrySeq,omitempty"`
	StepSeq       uint64         `json:"stepSeq,omitempty"`
	BlockSeq      uint64         `json:"blockSeq,omitempty"`
	Text          string         `json:"text,omitempty"`
	Entry         *session.Entry `json:"entry,omitempty"`
	Tool          *ToolEvent     `json:"tool,omitempty"`
	Status        RunStatus      `json:"status,omitempty"`
	Error         string         `json:"error,omitempty"`
}
