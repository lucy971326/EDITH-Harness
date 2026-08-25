package agents

import (
	"fmt"

	"harness/core"
	"harness/environment"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

// Plugin 把 Agent 管理处装进 App（能力名 "agents"）。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "agents"
}

// Start 领取会话运行所需能力，再登记运行时管理入口。
func (Plugin) Start(app *core.App) error {
	store, err := session.Get(app)
	if err != nil {
		return fmt.Errorf("agents 要先有账本管家（sessions）：%w", err)
	}
	toolsReg, err := tools.Get(app)
	if err != nil {
		return fmt.Errorf("agents 要先有工具登记处（tools）：%w", err)
	}
	projectService, err := projects.Get(app)
	if err != nil {
		return fmt.Errorf("agents 要先有项目管理处（projects）：%w", err)
	}
	presetService, err := presets.Get(app)
	if err != nil {
		return fmt.Errorf("agents 要先有模式管理处（presets）：%w", err)
	}
	provider, err := environment.Get(app)
	if err != nil {
		return fmt.Errorf("agents 要先有执行环境（environment）：%w", err)
	}
	service := newRegistry(app, store, projectService, presetService, provider, toolsReg)
	app.RegisterService("agents", service)
	app.OnCleanup(service.CloseAll)
	return nil
}

// Get 从 App 取 Agent 管理入口。
func Get(app *core.App) (Service, error) {
	return core.Resolve[Service](app, "agents")
}
