package commands

import "harness/core"

// Plugin 把一个空的命令登记处装进场地（能力名 "commands"）。
// 命令是各自的插件：Start 里 Get(app) 拿到登记处，再 Register 报名。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "commands"
}

// Start 往服务表登记一个空登记处。
func (Plugin) Start(app *core.App) error {
	app.RegisterService("commands", NewRegistry())
	return nil
}
