// Package terminal 提供最小的交互式终端界面插件。
package terminal

import (
	"fmt"
	"io"
	"os"

	"harness/agents"
	"harness/core"
	"harness/llm"
	"harness/tools"
)

// Plugin 把终端界面登记为当前 ui；测试可替换输入输出。
type Plugin struct {
	Input  io.Reader
	Output io.Writer
}

// Name 返回插件名。
func (Plugin) Name() string {
	return "ui-terminal"
}

// Start 领取管理、模型和工具能力，再登记终端界面启动入口。
func (p Plugin) Start(app *core.App) error {
	roster, err := agents.Get(app)
	if err != nil {
		return fmt.Errorf("terminal UI 要先有 Agent 管理处（agents）：%w", err)
	}
	models, err := llm.Get(app)
	if err != nil {
		return fmt.Errorf("terminal UI 要先有模型插座（llm）：%w", err)
	}
	registry, err := tools.Get(app)
	if err != nil {
		return fmt.Errorf("terminal UI 要先有工具登记处（tools）：%w", err)
	}
	if p.Input == nil {
		p.Input = os.Stdin
	}
	if p.Output == nil {
		p.Output = os.Stdout
	}
	service := newService(app, roster, models, registry, p.Input, p.Output)
	app.RegisterService("ui", service)
	return nil
}
