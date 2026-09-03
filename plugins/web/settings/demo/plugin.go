// Package demo 提供验证 Web 公共设置插槽 settings.section 边界的演示插件。
package demo

import (
	"harness/kernel/host"
	"harness/surface/web"
)

// 活对象。Plugin 在启动时向 Web 登记 demo 设置栏目及保存路由。
type Plugin struct {
	state *state
}

// New 造 demo 设置插件。
func New() *Plugin {
	return &Plugin{
		state: &state{
			nickname: "Explorer",
		},
	}
}

func (p *Plugin) Name() string { return "settings-demo" }

func (p *Plugin) Start(h *host.Host) error {
	webService, err := host.Resolve[web.Service](h, "web")
	if err != nil {
		return err
	}
	err = webService.RegisterSettingsSection(newDemoSection(p.state))
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("POST /settings/demo", newSaveHandler(p.state))
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	return nil
}
