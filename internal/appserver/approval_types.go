package appserver

import (
	"harness/internal/approvals"
	"harness/internal/permissions"
)

// 数据。读取全局智能审批设置。
type ApprovalSettingsParams struct{}

// 数据。本机用户订阅全部待审批，包含子 Agent 的来源身份。
type ApprovalSubscribeParams struct{}

// 数据。待审批初始快照；后续通知也是完整列表。
type ApprovalSubscribeResult struct {
	SubscriptionID string              `json:"subscriptionID"`
	Pending        []approvals.Pending `json:"pending"`
}

// 数据。只允许回答原申请，不能替换操作或修改权限。
type ApprovalRespondParams struct {
	RequestID string               `json:"requestID" jsonschema:"minLength=1"`
	Decision  permissions.Decision `json:"decision"`
}

// 数据。回答成功；申请结果以订阅投影为准。
type ApprovalRespondResult struct{}

// 数据。权限模式目录请求。
type PermissionModesParams struct{}

// 数据。后端提供的权限模式与可用性。
type PermissionModesResult struct {
	Modes []permissions.ModeChoice `json:"modes"`
}
