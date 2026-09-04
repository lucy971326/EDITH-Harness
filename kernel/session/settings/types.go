// Package settings 定义会话旁路设置。
package settings

// 数据。一本会话的旁路设置，不进账本。
type SessionSettings struct {
	AgentID         string `json:"agentID"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
	Workspace       string `json:"workspace"`
}

// 契约。按 session 读写 SessionSettings。
type SessionSettingsStore interface {
	For(sessionID string) (SessionSettings, error)
	Put(sessionID string, settings SessionSettings) error
	UsesAgent(agentID string) (bool, error)
}
