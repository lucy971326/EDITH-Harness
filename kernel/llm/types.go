package llm

import (
	"github.com/zendev-sh/goai/provider"

	"harness/kernel/session"
)

// Input 是 Runner 已准备好的本轮输入。
type Input struct {
	System  string
	History []session.Message
	Tools   []provider.ToolDefinition
}

// Client 用启动时读取的本机配置和模型定义发起模型调用。
type Client struct {
	config config
	models map[string]model
}
