// Package harness 定义 Harness 产品业务。
package harness

import (
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 产品进程内调用：操作输入

// 数据。启动新一轮聊天所需的已解析输入。
type RunInput struct {
	SessionID       string
	ExpectedRunID   string
	AgentID         string
	Model           string
	ReasoningEffort string
	Message         session.UserMessage
}

// 数据。分叉一段已完成助手回答所需的可信账本定位信息。
type ForkInput struct {
	SessionID       string
	RunID           string
	BoundaryEntryID string
}

// 产品进程内调用：查询结果

// 数据。一场会话的元数据与运行设置。
type SessionInfo struct {
	Meta     session.SessionMeta
	Settings settings.SessionSettings
}

// 数据。浏览器恢复聊天运行视图所需的耐久事实、运行状态与更新边界。
type Snapshot struct {
	Entries   []session.Entry   `json:"entries"`
	Runs      []runner.RunState `json:"runs"`
	UpdateSeq uint64            `json:"updateSeq"`
	SeqEpoch  string            `json:"seqEpoch"`
}
