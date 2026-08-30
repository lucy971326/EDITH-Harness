package agents

import (
	"harness/kernel/host"
	"harness/kernel/loops"
	"harness/kernel/skills"
	"harness/kernel/tools"
)

// 活对象。把 Agent 设置服务挂到 Host 的 agents 键。
type Plugin struct {
	service *Service
}

// NewPlugin 造 Agent 设置插件。
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "agents" }

func (p *Plugin) Start(h *host.Host) error {
	store, err := host.Resolve[AgentStore](h, "agentStore")
	if err != nil {
		return err
	}
	loopRegistry, err := host.Resolve[loops.Loops](h, "loops")
	if err != nil {
		return err
	}
	toolRegistry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		return err
	}
	skillRegistry, err := host.Resolve[skills.Skills](h, "skills")
	if err != nil {
		return err
	}
	p.service, err = NewService(store, loopRegistry, toolRegistry, skillRegistry)
	if err != nil {
		return err
	}
	if err := h.RegisterService("agents", p.service); err != nil {
		p.service = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.service = nil
	return nil
}
