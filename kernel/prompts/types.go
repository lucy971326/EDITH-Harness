// Package prompts 定义提示词登记处的契约。
package prompts

import (
	"context"

	"harness/kernel/session/settings"
)

// 数据。一段可按本轮 SessionSettings 生成的提示词。
type Part struct {
	Name   string
	Order  int
	Render func(context.Context, settings.SessionSettings) (string, error)
}

// 契约。提示词登记处提供的操作。
type Prompts interface {
	Register(part Part) (unregister func(), err error)
	Assemble(ctx context.Context, config settings.SessionSettings, local ...Part) (string, error)
}
