package appserver

import (
	"time"

	"harness/kernel/session/settings"
	"harness/products/harness"
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

// 对外会话接口：按会话 ID 操作

// 数据。查询、快照、订阅和停止共用的会话定位参数。
type SessionIDParams struct {
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

// 对外运行接口：本步先接通文字发送，图片随完整 API 迁移。

// 数据。闲时启动、忙时插话；忙时不修改当前运行设置。
type SendParams struct {
	SessionID       string `json:"sessionID" jsonschema:"minLength=1"`
	Text            string `json:"text" jsonschema:"minLength=1"`
	AgentID         string `json:"agentID,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	ExpectedRunID   string `json:"expectedRunID,omitempty" jsonschema:"minLength=1"`
}

// 数据。输入已接受；完整结果通过订阅或快照取得，不自动重试。
type SendResult struct {
	Mode string `json:"mode" jsonschema:"enum=started,enum=steered"`
}

// 数据。停止请求返回空对象，不等待整轮收尾。
type StopResult struct{}

// 数据。订阅先登记，再读快照；后续通知可能与快照重叠，账本按 Entry.ID 去重。
type SubscribeResult struct {
	SubscriptionID string           `json:"subscriptionID"`
	Snapshot       harness.Snapshot `json:"snapshot"`
}
