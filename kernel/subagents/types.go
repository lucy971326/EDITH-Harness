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

// 数据。单轮运行记录。
type TurnRecord struct {
	Turn          int        `json:"turn"`
	RunID         string     `json:"runID,omitempty"`
	Status        TaskStatus `json:"status"`
	ResultEntryID string     `json:"resultEntryID,omitempty"`
	Error         string     `json:"error,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// 数据。协作通知与投递确认；消息正文从子账本读取。
type Notification struct {
	Delivered      bool       `json:"delivered,omitempty"`
	NotificationID string     `json:"notificationID"`
	TaskID         string     `json:"taskID"`
	ChildSessionID string     `json:"childSessionID"`
	RunID          string     `json:"runID"`
	Turn           int        `json:"turn"`
	Status         TaskStatus `json:"status"`
	ResultEntryID  string     `json:"resultEntryID,omitempty"`
	Error          string     `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// 数据。持久化任务记录。业务事实独立保存在 ~/.harness/subagents/tasks/<id>.json。
type Task struct {
	Version         int            `json:"version"`
	ID              string         `json:"id"`
	ParentSessionID string         `json:"parentSessionID"`
	ParentRunID     string         `json:"parentRunID,omitempty"`
	ChildSessionID  string         `json:"childSessionID"`
	AgentID         string         `json:"agentID"`
	Model           string         `json:"model"`
	ReasoningEffort string         `json:"reasoningEffort"`
	Workspace       string         `json:"workspace"`
	Description     string         `json:"description"`
	Status          TaskStatus     `json:"status"`
	Turn            int            `json:"turn"`
	CurrentRunID    string         `json:"currentRunID,omitempty"`
	ResultEntryID   string         `json:"resultEntryID,omitempty"`
	Error           string         `json:"error,omitempty"`
	Turns           []TurnRecord   `json:"turns,omitempty"`
	Notifications   []Notification `json:"notifications,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// 数据。查询投影：任务记录加按账本位置读取的最终正文，不写入任务存储。
type TaskView struct {
	Task
	Results []TurnResult `json:"results"`
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
