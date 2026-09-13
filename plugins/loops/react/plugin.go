// Package react 提供默认的 ReAct 运行范式插件。
package react

import (
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/tools"
)

// 活对象。启动时填入 React 运行范式。
type Plugin struct{}

// New 造 React Loop 插件。
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return "react" }

func (p *Plugin) Start(h *host.Host) error {
	client, err := host.Resolve[*llm.Client](h, "llm")
	if err != nil {
		return err
	}
	toolRegistry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		return err
	}
	loopRegistry, err := host.Resolve[loops.Loops](h, "loops")
	if err != nil {
		return err
	}
	err = loopRegistry.Register(&reactLoop{llm: client, tools: toolRegistry})
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	return nil
}
