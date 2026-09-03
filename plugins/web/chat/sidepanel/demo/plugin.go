// Package demo 提供验证 Chat 右侧面板登记处边界的演示插件。
package demo

import (
	"harness/kernel/host"
	"harness/plugins/web/chat"
)

// 活对象。Plugin 在启动时把 demo 面板填入 Chat 登记处。
type Plugin struct{}

// New 造 demo 面板插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "panel-demo" }

func (p *Plugin) Start(h *host.Host) error {
	service, err := host.Resolve[chat.Service](h, "chat")
	if err != nil {
		return err
	}
	return service.RegisterPanel(panel{})
}

func (p *Plugin) Close() error {
	return nil
}
