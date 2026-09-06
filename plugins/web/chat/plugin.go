// Package chat 提供默认 Chat Web 产品。
package chat

import (
	"context"
	"fmt"

	chatservice "harness/kernel/chat"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/runner"
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
	business *chatservice.Service
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
	p.business, err = host.Resolve[*chatservice.Service](h, "chatService")
	if err != nil {
		return fmt.Errorf("chat: resolve chat service: %w", err)
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
	hub := p.hub
	p.unlisten, err = p.business.SubscribeRun(func(_ context.Context, event runner.RunEvent) error {
		hub.publishRun(event)
		return nil
	})
	if err != nil {
		return fmt.Errorf("chat: subscribe run events: %w", err)
	}
	unlistenDock, err := events.Subscribe(eventRegistry, func(_ context.Context, event DockChanged) error {
		hub.publishDock(event)
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
	handler := newPageHandler(webService, chatProduct, p.business, p.hub, p.registry)
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
	err = webService.RegisterRoute("POST /chat/{sessionID}/commands/{command}", handler)
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
	p.business = nil
	p.hub = nil
	p.registry = nil
	p.unlisten = nil
	return nil
}
