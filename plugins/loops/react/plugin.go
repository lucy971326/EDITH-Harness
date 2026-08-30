// Package react 提供默认的 ReAct 运行范式插件。
package react

import (
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/tools"
)

// 活对象。React 插件自己的登记状态。
type Plugin struct {
	unregister func()
}

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
	unregister, err := loopRegistry.Register(&reactLoop{llm: client, tools: toolRegistry})
	if err != nil {
		return err
	}
	p.unregister = unregister
	return nil
}

func (p *Plugin) Close() error {
	if p.unregister != nil {
		p.unregister()
		p.unregister = nil
	}
	return nil
}
