// Package agents 提供 Agent 设置的 Web 管理页面。
package agents

import (
	"fmt"

	agentservice "harness/kernel/agents"
	"harness/kernel/host"
	"harness/surface/web"
)

// 活对象。Plugin 向 Web 设置页登记 Agent 管理栏目与路由。
type Plugin struct{}

// New 造 Agent 设置 Web 插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "settings-agents" }

func (p *Plugin) Start(h *host.Host) error {
	webService, err := host.Resolve[web.Service](h, "web")
	if err != nil {
		return fmt.Errorf("settings-agents: resolve web: %w", err)
	}
	agentService, err := host.Resolve[*agentservice.Service](h, "agents")
	if err != nil {
		return fmt.Errorf("settings-agents: resolve dependency: %w", err)
	}
	page := newPage(agentService)
	err = webService.RegisterSettingsSection(page)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("GET /settings/agents/{agentID}", page)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("POST /settings/agents", page)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("POST /settings/agents/{agentID}", page)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("POST /settings/agents/{agentID}/delete", page)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error { return nil }
