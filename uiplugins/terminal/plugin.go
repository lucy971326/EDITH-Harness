// Package terminal 保留暂停中的终端 UI 插件入口。
package terminal

import (
	"context"
	"fmt"

	"harness/core"
)

// Plugin 登记暂停提示，避免旧 TUI 继续依赖已经移除的架构。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "ui-terminal"
}

// Start 登记暂停中的终端入口。
func (Plugin) Start(app *core.App) error {
	app.RegisterService("ui", pausedService{})
	return nil
}

type pausedService struct{}

func (pausedService) Run(context.Context) error {
	return fmt.Errorf("TUI 已暂停；下一阶段将接入 Web UI")
}
