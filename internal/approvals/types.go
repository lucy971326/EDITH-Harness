// Package approvals 管理本次操作的权限申请、智能审核与人工回答。
package approvals

import (
	"encoding/json"

	"harness/internal/permissions"
)

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
	Request      permissions.ApprovalRequest `json:"request"`
	MCP          *MCPRequest                 `json:"mcp,omitempty"`
	ReviewReason string                      `json:"reviewReason,omitempty"`
}

// 数据。项目 Server 启用或一次 MCP Tool 调用的独立审批内容。
type MCPRequest struct {
	Kind      string          `json:"kind"`
	Workspace string          `json:"workspace"`
	Source    string          `json:"source,omitempty"`
	Digest    string          `json:"digest,omitempty"`
	Servers   []MCPServer     `json:"servers,omitempty"`
	Server    string          `json:"server,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// 数据。供用户确认的项目 MCP Server 启动目标，不包含环境变量或 HTTP Header。
type MCPServer struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// 数据。所有智能审批共用的审核配置；密钥不在此处。
type Settings struct {
	Engine          string `json:"engine" jsonschema:"enum=llm,enum=jev"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
}

// 数据。设置页投影；配置状态不包含密钥。
type SettingsView struct {
	Settings      Settings `json:"settings"`
	JevConfigured bool     `json:"jevConfigured"`
	Available     bool     `json:"available"`
}
