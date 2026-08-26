// Package web 提供 edith-harness 的本地网页工作台。
package web

import (
	"fmt"

	"harness/agents"
	"harness/commands"
	"harness/core"
	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

// Plugin 把本地 Web UI 装进 App（能力名 "ui"）。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "ui-web"
}

// Start 领取界面所需能力，登记网页入口和账本更新观察者。
func (Plugin) Start(app *core.App) error {
	projectService, err := projects.Get(app)
	if err != nil {
		return fmt.Errorf("Web UI 要先有项目管理处（projects）：%w", err)
	}
	presetService, err := presets.Get(app)
	if err != nil {
		return fmt.Errorf("Web UI 要先有 Agent 模式管理处（presets）：%w", err)
	}
	books, err := session.Get(app)
	if err != nil {
		return fmt.Errorf("Web UI 要先有账本管家（sessions）：%w", err)
	}
	agentService, err := agents.Get(app)
	if err != nil {
		return fmt.Errorf("Web UI 要先有 Agent 管理处（agents）：%w", err)
	}
	llmService, err := llm.Get(app)
	if err != nil {
		return fmt.Errorf("Web UI 要先有模型插座排（llm）：%w", err)
	}
	toolRegistry, err := tools.Get(app)
	if err != nil {
		return fmt.Errorf("Web UI 要先有工具登记处（tools）：%w", err)
	}
	commandService, err := commands.Get(app)
	if err != nil {
		return fmt.Errorf("Web UI 要先有命令登记处（commands）：%w", err)
	}

	updates := newUpdateHub()
	service := newService(projectService, presetService, books, agentService, llmService, toolRegistry, commandService, updates, newNativeDirectoryPicker())
	app.Subscribe(session.EventAppended, func(payload any) {
		appended, ok := payload.(session.Appended)
		if ok {
			updates.Publish(appended.SessionID, updateNotice{
				Chat:     true,
				Composer: appended.Event.Kind == session.KindTurnStart || appended.Event.Kind == session.KindTurnEnd,
			})
		}
	})
	app.Subscribe(agents.EventConversationError, func(payload any) {
		failed, ok := payload.(agents.ConversationError)
		if ok {
			updates.Publish(failed.SessionID, updateNotice{Chat: true, Composer: true})
		}
	})
	app.RegisterService("ui", service)
	app.OnCleanup(service.Close)
	app.OnCleanup(updates.Close)
	return nil
}
