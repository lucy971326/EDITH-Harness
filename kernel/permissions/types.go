// Package permissions 计算 Agent 的执行权限；不执行审批或操作系统隔离。
package permissions

import (
	"context"
	"encoding/json"
)

// 数据。会话选择的权限模式。
type Mode string

// 数据。可供用户选择的权限模式；可用性由后端决定。
type ModeChoice struct {
	ID        Mode   `json:"id"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
}

const (
	ReadOnly       Mode = "read_only"
	AskForApproval Mode = "ask_for_approval"
	ApproveForMe   Mode = "approve_for_me"
	FullAccess     Mode = "full_access"
)

// 数据。执行权限；所有模式都允许读取宿主当前用户可读的文件。
// WriteRoots 中每个根的 .git、.agents、.harness 默认只读，显式的内部可写根除外。
// 路径只是词法规则，执行层仍须处理符号链接和实际访问隔离。
type Policy struct {
	Unrestricted bool     `json:"unrestricted"`
	WriteRoots   []string `json:"writeRoots"`
	Network      bool     `json:"network"`
}

// 数据。一次操作申请增加的权限，本身不代表授权。
type ExtraPermissions struct {
	WriteRoots []string `json:"writeRoots,omitempty"`
	Network    bool     `json:"network,omitempty"`
}

// 数据。模式选择的审核者；不放进文件与网络权限里。
type ReviewerKind string

const (
	NoReviewer    ReviewerKind = "none"
	HumanReviewer ReviewerKind = "human"
	ModelReviewer ReviewerKind = "model"
)

// 数据。执行前对额外权限申请的判断。
type Requirement uint8

const (
	Deny Requirement = iota
	Allow
	Ask
)

// 数据。交给审核者的操作与权限；请求归属和等待状态由审批服务管理。
type ApprovalRequest struct {
	// 待执行操作，不是审核者可以改写的命令。
	ToolName  string          `json:"toolName"`
	Arguments json.RawMessage `json:"arguments"`
	Workdir   string          `json:"workdir"`
	Reason    string          `json:"reason"`

	Current   Policy           `json:"current"`
	Requested ExtraPermissions `json:"requested"`
}

// 数据。对原申请的批准或拒绝，不携带审核者自行扩大的权限。
type Decision struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

// 契约。人审核与模型审核共用的入口；取消或审核故障返回错误。
type Reviewer interface {
	// 审核原申请，不修改权限或执行操作。
	Review(context.Context, ApprovalRequest) (Decision, error)
}
