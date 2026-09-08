// Package harness 定义 Harness 产品业务与对外会话契约。
package harness

import (
	"time"

	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 对外会话接口：创建（结果使用共享的 SessionResult）

// 数据。创建会话的接口输入；目录可用性由产品检查。
type CreateParams struct {
	Workspace string `json:"workspace" jsonschema:"minLength=1"`
}

// 对外会话接口：列表

// 数据。列表接口不接受过滤参数。
type ListParams struct{}

// 数据。没有普通会话时返回空数组，不返回 null。
type ListResult struct {
	Sessions []SessionView `json:"sessions"`
}

// 对外会话接口：查询（结果使用共享的 SessionResult）

// 数据。查询普通会话的接口输入。
type GetParams struct {
	SessionID string `json:"sessionID" jsonschema:"minLength=1"`
}

// 对外会话接口：共享结果与会话数据

// 数据。创建与单个查询返回同一种会话封套。
type SessionResult struct {
	Session SessionView `json:"session"`
}

// 数据。对外会话投影；复用设置数据，不改变持久化格式。
type SessionView struct {
	SessionID string                   `json:"sessionID" jsonschema:"minLength=1"`
	Title     string                   `json:"title"`
	CreatedAt time.Time                `json:"createdAt"`
	Settings  settings.SessionSettings `json:"settings"`
}

// 产品进程内调用：操作输入

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

// 产品进程内调用：查询结果

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
