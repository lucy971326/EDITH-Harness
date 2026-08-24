package loop

import (
	"fmt"

	"harness/agents"
	"harness/core"
	"harness/llm"
	"harness/tools"
)

// Plugin 把会话搬运工登记到 agents 管理处。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "loop"
}

// Start 领取模型和工具，再登记自己是会话 Runner。
func (Plugin) Start(app *core.App) error {
	llmSvc, err := llm.Get(app)
	if err != nil {
		return fmt.Errorf("loop 要先有插座排（llm）：%w", err)
	}
	toolsReg, err := tools.Get(app)
	if err != nil {
		return fmt.Errorf("loop 要先有工具登记处（tools）：%w", err)
	}
	service, err := agents.Get(app)
	if err != nil {
		return fmt.Errorf("loop 要先有 Agent 管理处（agents）：%w", err)
	}
	runner := NewRunner(llmSvc, toolsReg)
	unregister, err := service.RegisterRunner(runner)
	if err != nil {
		return fmt.Errorf("loop 登记会话搬运工失败：%w", err)
	}
	app.OnCleanup(unregister)
	return nil
}
