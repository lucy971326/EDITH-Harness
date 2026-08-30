// Package prompts 定义提示词登记处的契约。
package prompts

import (
	"context"

	"harness/kernel/kinds"
)

// 数据。一段可按本轮 Setup 生成的提示词。
type Part struct {
	Name   string
	Order  int
	Render func(context.Context, kinds.Setup) (string, error)
}

// 契约。提示词登记处提供的操作。
type Prompts interface {
	Register(part Part) (unregister func(), err error)
	Assemble(ctx context.Context, setup kinds.Setup, local ...Part) (string, error)
}
