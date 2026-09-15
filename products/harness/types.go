// Package harness 定义 Harness 产品业务。
package harness

import (
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/subagents"
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

// 数据。Subagent 页面可见的任务身份与当前状态，不暴露子 Session ID。
type SubagentInfo struct {
	TaskID          string               `json:"taskID"`
	TaskName        string               `json:"taskName"`
	Description     string               `json:"description"`
	AgentID         string               `json:"agentID"`
	Model           string               `json:"model"`
	ReasoningEffort string               `json:"reasoningEffort"`
	Workspace       string               `json:"workspace"`
	Status          subagents.TaskStatus `json:"status"`
	Turn            int                  `json:"turn"`
	CurrentRunID    string               `json:"currentRunID,omitempty"`
	Error           string               `json:"error,omitempty"`
}

// 数据。Subagent 页面一次同步需要的任务信息与运行快照。
type SubagentSnapshot struct {
	Task     SubagentInfo `json:"task"`
	Snapshot Snapshot     `json:"snapshot"`

	ChildSessionID string `json:"-"`
}
