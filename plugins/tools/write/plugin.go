// Package write 提供写文件工具插件。
package write

import (
	"harness/kernel/host"
	"harness/kernel/machine"
	"harness/kernel/tools"
)

// 活对象。启动时填入 write 工具。
type Plugin struct{}

// New 造 write 工具插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "write" }

func (p *Plugin) Start(h *host.Host) error {
	m, err := host.Resolve[machine.Machine](h, "machine")
	if err != nil {
		return err
	}
	registry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		return err
	}
	err = registry.Register(newTool(m))
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	return nil
}
