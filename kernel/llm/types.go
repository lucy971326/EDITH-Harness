package llm

import (
	"github.com/zendev-sh/goai/provider"

	"harness/kernel/session"
)

// 数据。Runner 已准备好的本轮输入。
type Input struct {
	System  string
	History []session.Message
	Tools   []provider.ToolDefinition
}
