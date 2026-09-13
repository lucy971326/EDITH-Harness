// Package subagents 定义子会话委派服务与独立存储。
package subagents

import (
	"errors"
	"time"

	"harness/kernel/agents"
	"harness/kernel/llm"
)

var (
	ErrClosed             = errors.New("subagents: service is closed")
	ErrFamilyStopped      = errors.New("subagents: this parent collaboration was stopped")
	ErrTaskStopped        = errors.New("subagents: this child run was stopped")
	ErrTaskNotFound       = errors.New("subagents: task not found")
	ErrParentRequired     = errors.New("subagents: parent session and run id are required")
	ErrDescriptionEmpty   = errors.New("subagents: description cannot be empty")
	ErrNestedDelegation   = errors.New("subagents: nested delegation not allowed (caller is already a child)")
	ErrOwnershipMismatch  = errors.New("subagents: task does not belong to specified parent session")
	ErrTaskNotUsable      = errors.New("subagents: child session or settings incomplete")
	ErrPersistFailed      = errors.New("subagents: task state could not be reliably persisted")
	ErrUnsupportedVersion = errors.New("subagents: unsupported task version")
	ErrInvalidTaskData    = errors.New("subagents: invalid task data")
)

// 数据。委派任务状态。
type TaskStatus string

const (
	StatusPending     TaskStatus = "pending"
	StatusRunning     TaskStatus = "running"
	StatusCompleted   TaskStatus = "completed"
	StatusFailed      TaskStatus = "failed"
	StatusCancelled   TaskStatus = "cancelled"
	StatusInterrupted TaskStatus = "interrupted"
)

// 数据。从子 Session 运行记录派生的单轮状态。
type TurnRecord struct {
	Turn          int        `json:"turn"`
	RunID         string     `json:"runID,omitempty"`
	Status        TaskStatus `json:"status"`
	ResultEntryID string     `json:"resultEntryID,omitempty"`
	Error         string     `json:"error,omitempty"`
}

// 数据。从子 Run 派生的协作通知；是否投递以父账本为准。
type Notification struct {
	NotificationID string     `json:"notificationID"`
	TaskID         string     `json:"taskID"`
	ChildSessionID string     `json:"childSessionID"`
	RunID          string     `json:"runID"`
	Turn           int        `json:"turn"`
	Status         TaskStatus `json:"status"`
	ResultEntryID  string     `json:"resultEntryID,omitempty"`
	Error          string     `json:"error,omitempty"`
}

// 数据。持久化的父子 Session 关系；不复制设置、运行和账本事实。
type TaskRecord struct {
	Version         int    `json:"version"`
	ID              string `json:"id"`
	ParentSessionID string `json:"parentSessionID"`
	ChildSessionID  string `json:"childSessionID"`
	Description     string `json:"description"`
}

// 数据。关系、子 Session 设置、运行与账本的只读查询投影。
type TaskView struct {
	ID              string `json:"id"`
	ParentSessionID string `json:"parentSessionID"`
	ChildSessionID  string `json:"childSessionID"`
	Description     string `json:"description"`

	AgentID         string `json:"agentID,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	Workspace       string `json:"workspace,omitempty"`

	Status        TaskStatus     `json:"status"`
	Turn          int            `json:"turn"`
	CurrentRunID  string         `json:"currentRunID,omitempty"`
	ResultEntryID string         `json:"resultEntryID,omitempty"`
	Error         string         `json:"error,omitempty"`
	Turns         []TurnRecord   `json:"turns"`
	Results       []TurnResult   `json:"results"`
	Notifications []Notification `json:"notifications"`
}

// 数据。一轮已保存结果的正文查询，不包含中途推理或工具消息。
type TurnResult struct {
	Turn    int    `json:"turn"`
	RunID   string `json:"runID"`
	EntryID string `json:"entryID"`
	Text    string `json:"text"`
}

// 数据。创建子任务的输入。
type SpawnInput struct {
	ParentSessionID string
	ParentRunID     string
	AgentID         string
	Model           string
	ReasoningEffort string
	Description     string
}

// 数据。Spawn 的返回结果。
type SpawnResult struct {
	TaskID         string `json:"taskID"`
	ChildSessionID string `json:"childSessionID"`
	RunID          string `json:"runID"`
}

// 数据。Send 的返回结果。
type SendResult struct {
	Turn    int    `json:"turn"`
	RunID   string `json:"runID"`
	Steered bool   `json:"steered"`
}

// 数据。系统可选 Agent 与模型。
type OptionsResult struct {
	Agents []agents.Agent    `json:"agents"`
	Models []llm.ModelChoice `json:"models"`
}

// 数据。Wait 查询与等待结果。
type WaitResult struct {
	NotificationID string     `json:"notificationID,omitempty"`
	TaskID         string     `json:"taskID"`
	Status         TaskStatus `json:"status"`
	Turn           int        `json:"turn"`
	RunID          string     `json:"runID"`
	ResultEntryID  string     `json:"resultEntryID,omitempty"`
	Error          string     `json:"error,omitempty"`
}

// 数据。等待指定任务的新完成通知；已见 ID 由调用者显式传回，不确认通知投递。
type WaitInput struct {
	TaskIDs             []string
	SeenNotificationIDs []string
	Timeout             time.Duration
	InputSignal         <-chan struct{}
}

// 数据。等待结束原因及状态快照；正文仍经 Runner 的协作消息路径交付。
type WaitResponse struct {
	Reason        string       `json:"reason"`
	Tasks         []WaitResult `json:"tasks"`
	Notifications []WaitResult `json:"notifications"`
}
