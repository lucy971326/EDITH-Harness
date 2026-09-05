package runner

import (
	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/tools"
)

// 活对象。把 Runner 挂到 Host 的 runner 键。
type Plugin struct {
	runner *Runner
}

// NewPlugin 造 Runner 插件。
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "runner" }

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
	loopRegistry, err := host.Resolve[loops.Loops](h, "loops")
	if err != nil {
		return err
	}
	eventRegistry, err := host.Resolve[*events.Registry](h, "events")
	if err != nil {
		return err
	}
	llmClient, err := host.Resolve[*llm.Client](h, "llm")
	if err != nil {
		return err
	}
	toolRegistry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		return err
	}
	p.runner, err = NewRunner(sessions, settingsStore, agentService, loopRegistry, eventRegistry, llmClient, toolRegistry)
	if err != nil {
		return err
	}
	err = h.RegisterService("runner", p.runner)
	if err != nil {
		p.runner = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	if p.runner != nil {
		p.runner.close()
	}
	p.runner = nil
	return nil
}
