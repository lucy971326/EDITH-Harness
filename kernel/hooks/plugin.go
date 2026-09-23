package hooks

import (
	"harness/kernel/host"
	"harness/kernel/machine"
	"harness/kernel/persist"
	"harness/kernel/tools"
)

// 活对象。组装 Hook 服务并接入工具登记处。
type Plugin struct{}

func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "hooks" }

func (p *Plugin) Start(h *host.Host) error {
	files, err := host.Resolve[*persist.Files](h, "persist")
	if err != nil {
		return err
	}
	filesystem, err := host.Resolve[machine.FileSystem](h, "machine")
	if err != nil {
		return err
	}
	registry, err := host.Resolve[*tools.Registry](h, "tools")
	if err != nil {
		return err
	}
	service, err := NewService(files, filesystem)
	if err != nil {
		return err
	}
	if err := h.RegisterService("hooks", service); err != nil {
		return err
	}
	registry.SetPreToolUse(service)
	return nil
}

func (p *Plugin) Close() error { return nil }
