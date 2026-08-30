package loops

import "harness/kernel/host"

// 活对象。把空运行范式登记处挂到 Host 的 loops 键。
type Plugin struct {
	registry *Registry
}

// NewPlugin 造运行范式登记处插件。
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "loops" }

func (p *Plugin) Start(h *host.Host) error {
	p.registry = NewRegistry()
	err := h.RegisterService("loops", p.registry)
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
