package subagents

import (
	"fmt"
	"path/filepath"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 活对象。将 Subagents 服务挂到 Host 的 subagents 键。
type Plugin struct {
	dir       string
	subagents *Subagents
}

// NewPlugin 创建 Subagents 插件，接收数据根目录（通常为 ~/.harness）。
func NewPlugin(dir string) *Plugin {
	return &Plugin{dir: dir}
}

func (p *Plugin) Name() string { return "subagents" }

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

	subagentsDir := filepath.Join(p.dir, "subagents")
	eventRegistry, err := host.Resolve[*events.Registry](h, "events")
	if err != nil {
		return err
	}
	s, err := NewSubagents(sessions, settingsStore, agentService, modelClient, runService, eventRegistry, subagentsDir)
	if err != nil {
		return fmt.Errorf("subagents plugin: new subagents: %w", err)
	}

	err = h.RegisterService("subagents", s)
	if err != nil {
		_ = s.Close()
		return err
	}
	p.subagents = s
	return nil
}

func (p *Plugin) Close() error {
	if p.subagents != nil {
		err := p.subagents.Close()
		p.subagents = nil
		return err
	}
	return nil
}
