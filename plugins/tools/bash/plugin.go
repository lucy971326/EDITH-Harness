// Package bash 提供 bash 命令工具插件。
package bash

import (
	"harness/kernel/host"
	"harness/kernel/machine"
	"harness/kernel/tools"
)

// 活对象。启动时填入 bash 工具。
type Plugin struct{}

// New 造 bash 工具插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "bash" }

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
