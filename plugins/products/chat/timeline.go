package chat

import (
	"harness/kernel/runner"
	"harness/kernel/session"
)

// 数据。Chat 恢复当前分叉耐久时间线的 JSON 快照。
type timelineSnapshot struct {
	Entries []session.Entry   `json:"entries"`
	Runs    []runner.RunState `json:"runs"`
}

// 数据。Chat 通过 SSE 收到的一条时间线变化。
type timelineEvent struct {
	Kind          runner.RunEventKind `json:"kind"`
	RunID         string              `json:"runID"`
	AfterEntrySeq uint64              `json:"afterEntrySeq,omitempty"`
	StepSeq       uint64              `json:"stepSeq,omitempty"`
	BlockSeq      uint64              `json:"blockSeq,omitempty"`
	Text          string              `json:"text,omitempty"`
	Entry         *session.Entry      `json:"entry,omitempty"`
	Tool          *runner.ToolEvent   `json:"tool,omitempty"`
	Status        runner.RunStatus    `json:"status,omitempty"`
	Error         string              `json:"error,omitempty"`
}

func timelineEventFromRun(event runner.RunEvent) timelineEvent {
	return timelineEvent{
		Kind:          event.Kind,
		RunID:         event.RunID,
		AfterEntrySeq: event.AfterEntrySeq,
		StepSeq:       event.StepSeq,
		BlockSeq:      event.BlockSeq,
		Text:          event.Text,
		Entry:         event.Entry,
		Tool:          event.Tool,
		Status:        event.Status,
		Error:         event.Error,
	}
}
