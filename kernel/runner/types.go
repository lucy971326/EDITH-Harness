// Package runner 定义一轮对话运行外壳。
package runner

import "harness/kernel/session"

// 数据。一条 Runner 对外事件的种类。表面不依赖 Loop 的内部事件。
type RunEventKind string

const (
	RunStarted     RunEventKind = "run-started"
	MessageStarted RunEventKind = "message-started"
	TextDelta      RunEventKind = "text-delta"
	ReasoningDelta RunEventKind = "reasoning-delta"
	ToolStarted    RunEventKind = "tool-started"
	ToolFinished   RunEventKind = "tool-finished"
	Message        RunEventKind = "message"
	ContextUsage   RunEventKind = "usage"
	RunEnded       RunEventKind = "run-ended"
)

// 数据。一轮 Run 的结束状态。
type RunStatus string

const (
	RunRunning     RunStatus = "running"
	RunSucceeded   RunStatus = "success"
	RunCancelled   RunStatus = "cancelled"
	RunFailed      RunStatus = "failed"
	RunInterrupted RunStatus = "interrupted"
)

// 数据。异步 Run 的最终稳定结果。
type RunResult struct {
	RunID  string
	Status RunStatus
	Err    error
}

// 数据。一场 Run 尚未落账的生成草稿；Blocks 是副本。
type RunDraft struct {
	EntryID       string          `json:"entryID"`
	AfterEntrySeq uint64          `json:"afterEntrySeq"`
	Blocks        []session.Block `json:"blocks"`
}

// 数据。一场 Run 的可恢复运行事实，含状态与未落账草稿。
type RunState struct {
	RunID         string     `json:"runID"`
	AfterEntrySeq uint64     `json:"afterEntrySeq"`
	Status        RunStatus  `json:"status"`
	Error         string     `json:"error,omitempty"`
	Drafts        []RunDraft `json:"drafts,omitempty"`
}

// 数据。一本会话的账本、运行状态和可比较的更新边界。
type SessionView struct {
	Entries   []session.Entry `json:"entries"`
	Runs      []RunState      `json:"runs"`
	UpdateSeq uint64          `json:"updateSeq"`
	SeqEpoch  string          `json:"seqEpoch"`
}

// 数据。工具实时事件需要的稳定事实。
type ToolEvent struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsError bool   `json:"isError"`
}

// 数据。最近一次模型调用的输入占用。不进账本。
type Usage struct {
	InputTokens     int `json:"inputTokens"`
	CacheReadTokens int `json:"cacheReadTokens"`
	ContextWindow   int `json:"contextWindow"`
}

// 数据。Runner 发布给界面的一条本轮事件。
type RunEvent struct {
	SessionID     string         `json:"sessionID"`
	RunID         string         `json:"runID"`
	Kind          RunEventKind   `json:"kind"`
	EntryID       string         `json:"entryID,omitempty"`
	AfterEntrySeq uint64         `json:"afterEntrySeq,omitempty"`
	BlockSeq      uint64         `json:"blockSeq,omitempty"`
	Text          string         `json:"text,omitempty"`
	Entry         *session.Entry `json:"entry,omitempty"`
	Tool          *ToolEvent     `json:"tool,omitempty"`
	Usage         *Usage         `json:"usage,omitempty"`
	Status        RunStatus      `json:"status,omitempty"`
	Error         string         `json:"error,omitempty"`
	UpdateSeq     uint64         `json:"updateSeq"`
	SeqEpoch      string         `json:"seqEpoch"`
}
