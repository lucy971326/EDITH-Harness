// Package agents 定义 Agent 设置服务的契约。
package agents

import (
	"harness/kernel/agents/config"
	"harness/kernel/loops"
	"harness/kernel/skills"
	"harness/kernel/tools"
)

const DefaultID = "default"

// 数据。Alias 到 Agent 设置域的纯数据，供 UI 和 Agent 服务共同使用。
type Agent = config.Agent

// 数据。Agent 设置界面可展示的候选项。
type Choices struct {
	Loops  []loops.Definition
	Tools  []tools.Definition
	Skills []skills.Skill
}

// 数据。Runner 使用的一份已准备 Agent。
type PreparedAgent struct {
	Kind         string
	Tools        []string
	SystemPrompt string
}

// 契约。Alias 到 Agent 设置域的持久化读写契约。
type AgentStore = config.Store
