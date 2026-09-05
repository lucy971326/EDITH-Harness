// Package chat 提供默认 Chat Web 产品。
package chat

import (
	"context"
	"fmt"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/surface/web"
	"harness/surface/web/ui"
)

var chatProduct = web.Product{
	ID:       "chat",
	Name:     "对话",
	Icon:     ui.IconChat,
	BasePath: "/chat",
	Order:    10,
}

// 活对象。Plugin 把 Chat 产品及其路由填入 Web 登记处。
type Plugin struct {
	sessions *session.Store
	settings settings.SessionSettingsStore
	agents   *agents.Service
	runner   *runner.Runner
	models   *llm.Client
	hub      *eventHub
	registry *registry
	unlisten func()
}

// NewPlugin 创建 Chat 产品插件。
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "chat" }

func (p *Plugin) Start(h *host.Host) error {
	webService, err := host.Resolve[web.Service](h, "web")
	if err != nil {
		return fmt.Errorf("chat: resolve web: %w", err)
	}
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		return fmt.Errorf("chat: resolve sessions: %w", err)
	}
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		return fmt.Errorf("chat: resolve session settings: %w", err)
	}
	p.sessions = sessions
	p.settings = settingsStore
	p.agents, err = host.Resolve[*agents.Service](h, "agents")
	if err != nil {
		return fmt.Errorf("chat: resolve agents: %w", err)
	}
	p.runner, err = host.Resolve[*runner.Runner](h, "runner")
	if err != nil {
		return fmt.Errorf("chat: resolve runner: %w", err)
	}
	p.models, err = host.Resolve[*llm.Client](h, "llm")
	if err != nil {
		return fmt.Errorf("chat: resolve llm: %w", err)
	}
	eventRegistry, err := host.Resolve[*events.Registry](h, "events")
	if err != nil {
		return fmt.Errorf("chat: resolve events: %w", err)
	}
	p.hub = newEventHub()
	p.registry = newRegistry()
	err = p.registry.RegisterMessageAction(copyMessageAction{})
	if err != nil {
		return err
	}
	err = h.RegisterService("chat", Service(p.registry))
	if err != nil {
		return err
	}
	p.unlisten, err = events.Subscribe(eventRegistry, func(_ context.Context, event runner.RunEvent) error {
		p.hub.publishRun(event)
		return nil
	})
	if err != nil {
		return fmt.Errorf("chat: subscribe run events: %w", err)
	}
	unlistenDock, err := events.Subscribe(eventRegistry, func(_ context.Context, event DockChanged) error {
		p.hub.publishDock(event)
		return nil
	})
	if err != nil {
		p.unlisten()
		p.unlisten = nil
		return fmt.Errorf("chat: subscribe dock events: %w", err)
	}
	unlistenRun := p.unlisten
	p.unlisten = func() {
		unlistenDock()
		unlistenRun()
	}
	err = webService.RegisterProduct(chatProduct)
	if err != nil {
		return err
	}
	staticHandler, err := chatStaticHandler()
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("GET /assets/chat/", staticHandler)
	if err != nil {
		return err
	}
	handler := newPageHandler(webService, chatProduct, sessions, settingsStore, p.agents, p.runner, p.models, p.hub, p.registry)
	err = webService.RegisterRoute("GET /chat", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("GET /chat/{sessionID}", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("POST /chat/projects", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("POST /chat/{sessionID}/sessions", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("GET /chat/{sessionID}/history", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("GET /chat/{sessionID}/suggestions", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("GET /chat/{sessionID}/events", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("POST /chat/{sessionID}/messages", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("POST /chat/{sessionID}/message-actions/{actionID}", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("POST /chat/{sessionID}/stop", handler)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("GET /chat/{sessionID}/panels/{panelType}", handler)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	if p.unlisten != nil {
		p.unlisten()
	}
	if p.hub != nil {
		p.hub.close()
	}
	p.sessions = nil
	p.settings = nil
	p.agents = nil
	p.runner = nil
	p.models = nil
	p.hub = nil
	p.registry = nil
	p.unlisten = nil
	return nil
}
