// Package agents 定义 Agent 设置服务的契约。
package agents

import (
	"harness/kernel/loops"
	"harness/kernel/tools"
)

const DefaultID = "default"

// 数据。一份用户可选择的 Agent 设置。
type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	SystemPrompt string   `json:"systemPrompt"`
	Tools        []string `json:"tools"`
}

// 数据。Agent 设置界面可展示的候选项。
type Choices struct {
	Loops []loops.Definition
	Tools []tools.Definition
}

// 数据。Runner 使用的一份已准备 Agent。
type PreparedAgent struct {
	Kind         string
	Tools        []string
	SystemPrompt string
}

// 契约。自建 Agent 的持久化读写。
type AgentStore interface {
	ListAgents() ([]Agent, error)
	ForAgent(id string) (Agent, error)
	PutAgent(agent Agent) error
	DeleteAgent(id string) error
}
