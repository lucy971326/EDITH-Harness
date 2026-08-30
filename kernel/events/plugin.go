package events

import "harness/kernel/host"

// 活对象。把空事件监听登记处挂到 Host 的 events 键。
type Plugin struct {
	registry *Registry
}

// NewPlugin 造事件登记处插件。
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "events" }

func (p *Plugin) Start(h *host.Host) error {
	p.registry = NewRegistry()
	err := h.RegisterService("events", p.registry)
	if err != nil {
		p.registry = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.registry = nil
	return nil
}
