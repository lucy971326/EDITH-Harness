package appserver

import (
	"time"

	"harness/kernel/runner"
	"harness/kernel/session/settings"
	"harness/products/harness"
)

// 数据。读取一个 Run Diff 文件的接口输入。
type ReadRunDiffParams struct {
	SessionID string `json:"sessionID" jsonschema:"minLength=1"`
	RunID     string `json:"runID" jsonschema:"minLength=1"`
	Path      string `json:"path" jsonschema:"minLength=1"`
}

// 数据。按版本安全撤销一个 Run Diff 文件的接口输入。
type RevertRunDiffParams struct {
	SessionID        string `json:"sessionID" jsonschema:"minLength=1"`
	RunID            string `json:"runID" jsonschema:"minLength=1"`
	Path             string `json:"path" jsonschema:"minLength=1"`
	ExpectedRevision uint64 `json:"expectedRevision" jsonschema:"minimum=1"`
}

// 数据。读取接口直接返回 Runner 的单文件事实。
type ReadRunDiffResult = runner.RunDiffFile

// 数据。撤销接口返回新的耐久 Diff 摘要；文件为空表示已全部撤销。
type RevertRunDiffResult = runner.RunDiffSummary

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

// 对外会话接口：下一轮设置

// 数据。只更新运行设置，工作区始终沿用当前会话。
type UpdateSettingsParams struct {
	SessionID       string `json:"sessionID" jsonschema:"minLength=1"`
	AgentID         string `json:"agentID" jsonschema:"minLength=1"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
}

// 对外会话接口：从完整回答分叉

// 数据。用回答所在 Run 与落账消息定位分叉边界。
type ForkParams struct {
	SessionID       string `json:"sessionID" jsonschema:"minLength=1"`
	RunID           string `json:"runID" jsonschema:"minLength=1"`
	BoundaryEntryID string `json:"boundaryEntryID" jsonschema:"minLength=1"`
}

// 对外运行接口：文字与压缩后的图片发送。

// 数据。Client 已压缩的一张图片；服务端仍会核对实际内容。
type ImageInput struct {
	MIME string `json:"mime" jsonschema:"enum=image/png,enum=image/jpeg,enum=image/webp"`
	Data string `json:"data" jsonschema:"minLength=1"`
}

// 数据。闲时启动、忙时插话；忙时不修改当前运行设置。
type SendParams struct {
	SessionID     string       `json:"sessionID" jsonschema:"minLength=1"`
	Text          string       `json:"text,omitempty"`
	Images        []ImageInput `json:"images,omitempty" jsonschema:"maxItems=4"`
	ExpectedRunID string       `json:"expectedRunID,omitempty" jsonschema:"minLength=1"`
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

// 对外 Subagent 接口：父会话与任务共同定位，绝不接受子 Session ID。

// 数据。定位父会话中的一个子任务。
type SubagentParams struct {
	ParentSessionID string `json:"parentSessionID" jsonschema:"minLength=1"`
	TaskID          string `json:"taskID" jsonschema:"minLength=1"`
}

// 数据。列出一个父会话的全部直接子任务。
type SubagentListParams struct {
	ParentSessionID string `json:"parentSessionID" jsonschema:"minLength=1"`
}

// 数据。子任务列表结果。
type SubagentListResult struct {
	Tasks []harness.SubagentInfo `json:"tasks"`
}

// 数据。子任务订阅先返回快照，后续复用 harness/run/event 通知。
type SubagentSubscribeResult struct {
	SubscriptionID string               `json:"subscriptionID"`
	Task           harness.SubagentInfo `json:"task"`
	Snapshot       harness.Snapshot     `json:"snapshot"`
}

// 数据。用户从子任务页面发送文字或图片。
type SubagentSendParams struct {
	ParentSessionID string       `json:"parentSessionID" jsonschema:"minLength=1"`
	TaskID          string       `json:"taskID" jsonschema:"minLength=1"`
	Text            string       `json:"text,omitempty"`
	Images          []ImageInput `json:"images,omitempty" jsonschema:"maxItems=4"`
}

// 数据。子任务发送已被接受。
type SubagentSendResult struct {
	Mode  string `json:"mode" jsonschema:"enum=started,enum=steered"`
	Turn  int    `json:"turn"`
	RunID string `json:"runID"`
}

// 数据。子任务空闲时可更新下一轮的模型和思考档位。
type SubagentSettingsParams struct {
	ParentSessionID string `json:"parentSessionID" jsonschema:"minLength=1"`
	TaskID          string `json:"taskID" jsonschema:"minLength=1"`
	Model           string `json:"model" jsonschema:"minLength=1"`
	ReasoningEffort string `json:"reasoningEffort" jsonschema:"minLength=1"`
}

// 数据。子任务设置更新后返回任务投影。
type SubagentSettingsResult struct {
	Task harness.SubagentInfo `json:"task"`
}

// 数据。读取一个子任务 Run Diff 文件。
type ReadSubagentRunDiffParams struct {
	ParentSessionID string `json:"parentSessionID" jsonschema:"minLength=1"`
	TaskID          string `json:"taskID" jsonschema:"minLength=1"`
	RunID           string `json:"runID" jsonschema:"minLength=1"`
	Path            string `json:"path" jsonschema:"minLength=1"`
}

// 数据。撤销一个子任务 Run Diff 文件。
type RevertSubagentRunDiffParams struct {
	ParentSessionID  string `json:"parentSessionID" jsonschema:"minLength=1"`
	TaskID           string `json:"taskID" jsonschema:"minLength=1"`
	RunID            string `json:"runID" jsonschema:"minLength=1"`
	Path             string `json:"path" jsonschema:"minLength=1"`
	ExpectedRevision uint64 `json:"expectedRevision" jsonschema:"minimum=1"`
}
