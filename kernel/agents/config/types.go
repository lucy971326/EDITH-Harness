// Package config 定义 Agent 设置的纯数据和持久化契约。
package config

// 数据。一份用户可选择的 Agent 设置。
type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	SystemPrompt string   `json:"systemPrompt"`
	Tools        []string `json:"tools"`
}

// 契约。自建 Agent 的持久化读写。
type Store interface {
	ListAgents() ([]Agent, error)
	ForAgent(id string) (Agent, error)
	PutAgent(agent Agent) error
	DeleteAgent(id string) error
}
