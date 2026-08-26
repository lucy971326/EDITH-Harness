package agents

import "harness/llm"

// StartInput 是新会话第一次启动时锁定的项目、模式和标题。
type StartInput struct {
	ProjectID string
	PresetID  string
	Title     string
	Model     llm.Selection
}
