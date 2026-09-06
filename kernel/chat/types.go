// Package chat 定义聊天业务服务。
package chat

import (
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 数据。一场会话的元数据与运行设置。
type SessionInfo struct {
	Meta     session.SessionMeta
	Settings settings.SessionSettings
}

// 数据。浏览器恢复聊天运行视图所需的耐久事实与活跃 Run。
type Snapshot struct {
	Entries []session.Entry   `json:"entries"`
	Runs    []runner.RunState `json:"runs"`
}

// 数据。启动新一轮聊天所需的已解析输入。
type RunInput struct {
	SessionID       string
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
