// Package exec 提供可继续交互的命令执行工具。
package exec

import (
	"harness/kernel/host"
	"harness/kernel/machine"
	"harness/kernel/tools"
)

// 活对象。启动时登记 exec_command 与 write_stdin。
type Plugin struct{}

// New 造长期进程工具插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "exec-tools" }

func (p *Plugin) Start(h *host.Host) error {
	processes, err := host.Resolve[machine.AgentProcesses](h, "machine")
	if err != nil {
		return err
	}
	paths, err := host.Resolve[machine.Machine](h, "machine")
	if err != nil {
		return err
	}
	registry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		return err
	}

	for _, tool := range toolEntries(processes, paths) {
		err = registry.Register(tool)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Plugin) Close() error { return nil }
