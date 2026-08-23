package agents

import (
	"fmt"

	"harness/core"
	"harness/session"
	"harness/tools"
)

// Plugin 把 Agent 管理处装进 App（能力名 "agents"）。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "agents"
}

// Start 领取档案、账本和工具登记处，再登记 Agent 管理能力。
func (Plugin) Start(app *core.App) error {
	store, err := session.Get(app)
	if err != nil {
		return fmt.Errorf("agents 要先有账本管家（sessions）：%w", err)
	}
	toolsReg, err := tools.Get(app)
	if err != nil {
		return fmt.Errorf("agents 要先有工具登记处（tools）：%w", err)
	}
	profiles, err := core.Resolve[ProfileStore](app, "agent-profiles")
	if err != nil {
		return fmt.Errorf("agents 要先有 Agent 档案库（agent-profiles）：%w", err)
	}
	service := newRegistry(app, store, profiles, toolsReg)
	app.RegisterService("agents", service)
	app.OnCleanup(service.CloseAll)
	return nil
}

// Get 从 App 取 Agent 管理入口。
func Get(app *core.App) (Service, error) {
	return core.Resolve[Service](app, "agents")
}
