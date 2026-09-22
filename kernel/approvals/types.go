// Package approvals 管理本次操作的权限申请与人工回答。
package approvals

import "harness/kernel/permissions"

// 数据。由执行链提供的申请归属，不接受模型填写。
type Identity struct {
	SessionID  string `json:"sessionID"`
	RunID      string `json:"runID"`
	ToolCallID string `json:"toolCallID"`
}

// 数据。一项等待用户回答的固定操作。
type Pending struct {
	ID string `json:"id"`
	Identity
	Request permissions.ApprovalRequest `json:"request"`
}
