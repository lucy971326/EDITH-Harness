// Package demo 提供验证 Chat 输入工具栏插槽边界的演示插件。
package demo

import (
	"harness/kernel/host"
	"harness/plugins/products/chat"
)

// 活对象。Plugin 在启动时把 demo 动作填入 Chat 登记处。
type Plugin struct{}

// New 造 demo 输入动作插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "composer-demo" }

func (p *Plugin) Start(h *host.Host) error {
	service, err := host.Resolve[chat.Service](h, "chat")
	if err != nil {
		return err
	}
	return service.RegisterComposerAction(composerAction{})
}

func (p *Plugin) Close() error {
	return nil
}
