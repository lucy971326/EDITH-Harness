package loop

import (
	"fmt"

	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

// Plugin 把名册装进场地（能力名 "agents"）。
// 装它之前，session、llm、tools 必须已就位——缺谁启动就报谁的错，逆序回滚。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "loop"
}

// Start 造名册挂进场地；名册跟着 App 一起收摊。
func (Plugin) Start(app *core.App) error {
	store, err := core.Resolve[*session.Store](app, "sessions")
	if err != nil {
		return fmt.Errorf("loop 要先有账本管家（sessions）：%w", err)
	}
	llmSvc, err := llm.Get(app)
	if err != nil {
		return fmt.Errorf("loop 要先有插座排（llm）：%w", err)
	}
	toolsReg, err := tools.Get(app)
	if err != nil {
		return fmt.Errorf("loop 要先有工具登记处（tools）：%w", err)
	}

	roster := NewRoster(app, store, llmSvc, toolsReg)
	app.RegisterService("agents", roster)
	app.OnCleanup(roster.CloseAll)
	return nil
}

// Get 从场地取名册。
func Get(app *core.App) (*Roster, error) {
	return core.Resolve[*Roster](app, "agents")
}
