package tools

import "harness/core"

// Plugin 把一个空的工具登记处装进场地（能力名 "tools"）。
// 工具是各自的插件：Start 里 Get(app) 拿到登记处，再 Register 报名。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "tools"
}

// Start 往服务表登记一个空登记处。
func (Plugin) Start(app *core.App) error {
	app.RegisterService("tools", NewRegistry())
	return nil
}

// Get 从场地取工具登记处。
func Get(app *core.App) (*Registry, error) {
	return core.Resolve[*Registry](app, "tools")
}
