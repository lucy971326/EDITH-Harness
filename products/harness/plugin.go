package harness

import (
	"harness/kernel/agents"
	"harness/kernel/commands"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/subagents"
)

// 活对象。安装 harnessProduct；只组装业务依赖。
type Plugin struct{ service *Product }

// NewPlugin 造 Harness 产品插件；不拥有内核或 app-server 的资源。
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "harness-product" }

func (p *Plugin) Start(h *host.Host) error {
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		return err
	}
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		return err
	}
	agentService, err := host.Resolve[*agents.Service](h, "agents")
	if err != nil {
		return err
	}
	modelClient, err := host.Resolve[*llm.Client](h, "llm")
	if err != nil {
		return err
	}
	runService, err := host.Resolve[*runner.Runner](h, "runner")
	if err != nil {
		return err
	}
	commandService, err := host.Resolve[commands.Commands](h, "commands")
	if err != nil {
		return err
	}
	subagentService, err := host.Resolve[*subagents.Subagents](h, "subagents")
	if err != nil {
		return err
	}
	p.service, err = New(sessions, settingsStore, agentService, modelClient, runService, commandService, subagentService)
	if err != nil {
		return err
	}
	err = h.RegisterService("harnessProduct", p.service)
	if err != nil {
		p.service = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error { p.service = nil; return nil }
