package chat

import (
	"harness/kernel/agents"
	"harness/kernel/commands"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 活对象。把 ChatService 挂到 Host 的 chatService 键。
type Plugin struct{ service *Service }

// NewPlugin 造 ChatService 插件。
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "chat-service" }

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
	eventRegistry, err := host.Resolve[*events.Registry](h, "events")
	if err != nil {
		return err
	}
	p.service, err = NewService(sessions, settingsStore, agentService, modelClient, runService, commandService, eventRegistry)
	if err != nil {
		return err
	}
	err = h.RegisterService("chatService", p.service)
	if err != nil {
		p.service = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error { p.service = nil; return nil }
