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

// 数据。待投递通知记录（本步仅持久化保存，不投递父账本）。
type Notification struct {
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
	TaskID         string
	ChildSessionID string
	RunID          string
}

// 数据。Send 的返回结果。
type SendResult struct {
	Turn    int
	RunID   string
	Steered bool
}

// 数据。系统可选 Agent 与模型。
type OptionsResult struct {
	Agents []agents.Agent
	Models []llm.ModelChoice
}

// 数据。Wait 查询与等待结果。
type WaitResult struct {
	TaskID        string
	Status        TaskStatus
	Turn          int
	RunID         string
	ResultEntryID string
	Error         string
}
