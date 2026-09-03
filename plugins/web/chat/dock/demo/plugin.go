// Package demo 提供验证 Chat Dock 插槽边界的动态演示插件。
package demo

import (
	"context"
	"fmt"
	"sync"

	"github.com/a-h/templ"

	"harness/kernel/events"
	"harness/kernel/host"
	"harness/plugins/web/chat"
)

// 活对象。Plugin 保存测试用的按会话计数，并把 demo 条目填入 Chat Dock。
type Plugin struct {
	mu      sync.Mutex
	values  map[string]int
	events  *events.Registry
	service chat.Service
}

// New 造动态 Dock 演示插件。
func New() *Plugin {
	return &Plugin{values: make(map[string]int)}
}

func (p *Plugin) Name() string { return "dock-demo" }

func (p *Plugin) Start(h *host.Host) error {
	service, err := host.Resolve[chat.Service](h, "chat")
	if err != nil {
		return fmt.Errorf("dock demo: resolve chat: %w", err)
	}
	eventRegistry, err := host.Resolve[*events.Registry](h, "events")
	if err != nil {
		return fmt.Errorf("dock demo: resolve events: %w", err)
	}
	p.service = service
	p.events = eventRegistry
	if err := service.RegisterDock(dock{plugin: p}); err != nil {
		p.service = nil
		p.events = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.mu.Lock()
	p.values = nil
	p.mu.Unlock()
	p.events = nil
	p.service = nil
	return nil
}

// Increment 改变一条测试状态，并通知该会话的 Dock 更新。
func (p *Plugin) Increment(ctx context.Context, sessionID string) error {
	p.mu.Lock()
	if p.values == nil {
		p.mu.Unlock()
		return fmt.Errorf("dock demo: closed")
	}
	p.values[sessionID]++
	p.mu.Unlock()
	return events.Publish(ctx, p.events, chat.DockChanged{SessionID: sessionID, DockID: "demo"})
}

func (p *Plugin) value(sessionID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.values[sessionID]
}

type dock struct {
	plugin *Plugin
}

func (dock) Definition() chat.DockDefinition {
	return chat.DockDefinition{ID: "demo", Name: "动态演示", Order: 10}
}

func (d dock) Render(context chat.DockContext) (templ.Component, error) {
	return content(d.plugin.value(context.SessionID)), nil
}
