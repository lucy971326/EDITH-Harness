// Package demo 保留 Chat 消息动作插槽的独立填充者形状。
package demo

import (
	"context"
	"fmt"

	"harness/kernel/host"
	"harness/plugins/web/chat"
)

// 活对象。Plugin 启动时把占位消息动作填入 Chat 登记处。
//
// 此插件不安装进 cmd/harness；它只保留未来独立消息动作的最小形状。
type Plugin struct{}

// New 创建占位消息动作插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "message-demo" }

func (p *Plugin) Start(h *host.Host) error {
	service, err := host.Resolve[chat.Service](h, "chat")
	if err != nil {
		return err
	}
	return service.RegisterMessageAction(action{})
}

func (p *Plugin) Close() error {
	return nil
}

// 活对象。action 是无业务行为的登记占位；未来独立动作从这里替换。
type action struct{}

func (action) Definition() chat.MessageActionDefinition {
	return chat.MessageActionDefinition{
		ID:      "demo",
		Name:    "示例",
		Icon:    "…",
		Order:   1000,
		Targets: []chat.MessageCardType{chat.MessageCardUser, chat.MessageCardAssistant},
	}
}

func (action) Execute(context.Context, chat.MessageActionContext) (chat.MessageActionResult, error) {
	return chat.MessageActionResult{}, fmt.Errorf("message demo: placeholder action has no behavior")
}
