// Package chat 提供默认 Chat Web 产品。
package chat

import (
	"fmt"

	"harness/kernel/host"
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
type Plugin struct{}

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
	err = webService.RegisterProduct(chatProduct)
	if err != nil {
		return err
	}
	err = webService.RegisterRoute("GET /chat", newPageHandler(webService, chatProduct))
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error { return nil }
