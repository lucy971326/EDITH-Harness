package llm

import "harness/core"

// Plugin 把一个空的插座排装进场地（能力名 "llm"）。
// 适配器是各自的插件：Start 里 Get(app) 拿到插座排，再 Register 往上插。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "llm"
}

// Start 往服务表登记一个空插座排。
func (Plugin) Start(app *core.App) error {
	app.RegisterService("llm", NewService())
	return nil
}

// Get 从场地取插座排。
func Get(app *core.App) (*Service, error) {
	return core.Resolve[*Service](app, "llm")
}
