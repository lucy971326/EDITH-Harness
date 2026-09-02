package llm

import (
	"harness/kernel/session"
	"harness/kernel/tools"
)

// 数据。当前本机配置可供会话选择的一种模型。
type ModelChoice struct {
	ID               string
	ReasoningEfforts []string
}

// 数据。一次模型调用使用的模型和思考档位。
type RunConfig struct {
	Model           string
	ReasoningEffort string
}

// 数据。一次模型调用的提示词、历史和工具定义。
type Input struct {
	System  string
	History []session.Message
	Tools   []tools.Definition
}
