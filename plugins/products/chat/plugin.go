// Package chat 提供默认 Chat Web 产品。
package chat

import (
	"fmt"

	"harness/kernel/host"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/surface/web"
)

var chatProduct = web.Product{
	ID:       "chat",
	Name:     "对话",
	Icon:     "▣",
	BasePath: "/chat",
	Order:    10,
}

// 活对象。Plugin 把 Chat 产品及其路由填入 Web 登记处。
type Plugin struct {
	sessions *session.Store
	settings settings.SessionSettingsStore
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
	err = webService.RegisterProduct(chatProduct)
	if err != nil {
		return err
	}
	handler := newPageHandler(webService, chatProduct, sessions, settingsStore)
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
	return nil
}

func (p *Plugin) Close() error {
	p.sessions = nil
	p.settings = nil
	return nil
}
