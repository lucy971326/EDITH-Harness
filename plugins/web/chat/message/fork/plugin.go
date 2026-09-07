// Package fork 为 Chat 已完成回答提供创建独立分叉会话的消息动作。
package fork

import (
	"context"
	"fmt"

	"harness/kernel/host"
	"harness/plugins/web/chat"
	harnessproduct "harness/products/harness"
)

// 活对象。Plugin 将分叉动作填入 Chat 的 message.actions 插槽。
type Plugin struct {
	business *harnessproduct.Product
}

// New 创建分叉消息动作插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "message-fork" }

func (p *Plugin) Start(h *host.Host) error {
	service, err := host.Resolve[chat.Service](h, "chat")
	if err != nil {
		return fmt.Errorf("message fork: resolve chat: %w", err)
	}
	p.business, err = host.Resolve[*harnessproduct.Product](h, "harnessProduct")
	if err != nil {
		return fmt.Errorf("message fork: resolve harness product: %w", err)
	}
	err = service.RegisterMessageAction(action{business: p.business})
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.business = nil
	return nil
}

// 活对象。action 从已完成的助手回答创建一场独立会话。
type action struct {
	business *harnessproduct.Product
}

func (action) Definition() chat.MessageActionDefinition {
	return chat.MessageActionDefinition{
		ID:      "fork",
		Name:    "分叉",
		Icon:    "⑂",
		Order:   20,
		Targets: []chat.MessageCardType{chat.MessageCardAssistant},
	}
}

func (a action) Execute(_ context.Context, input chat.MessageActionContext) (chat.MessageActionResult, error) {
	destinationID, err := a.business.Fork(harnessproduct.ForkInput{SessionID: input.SessionID, RunID: input.Target.RunID, BoundaryEntryID: input.Target.BoundaryEntryID})
	if err != nil {
		return chat.MessageActionResult{}, err
	}
	return chat.MessageActionResult{Redirect: "/chat/" + destinationID}, nil
}
