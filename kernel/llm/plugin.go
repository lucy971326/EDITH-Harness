package llm

import "harness/kernel/host"

var _ host.Plugin = (*Plugin)(nil)

// Plugin 构造 Service 并以 "llm" 挂到 Host。Adapter 不自行注册。
type Plugin struct {
	Config   Config
	Catalog  Catalog
	Adapters map[string]Adapter

	service *Service
}

func (p *Plugin) Name() string { return "llm" }

func (p *Plugin) Start(h *host.Host) error {
	service, err := New(p.Config, p.Catalog, p.Adapters)
	if err != nil {
		return err
	}
	if err := h.RegisterService("llm", service); err != nil {
		return err
	}
	p.service = service
	return nil
}

func (p *Plugin) Close() error {
	p.service = nil
	return nil
}
