// Package subagents 将子会话委派服务接入现有工具登记处。
package subagents

import (
	"harness/kernel/host"
	delegation "harness/kernel/subagents"
	"harness/kernel/tools"
)

// 活对象。启动时注册六个委派工具，不拥有子 Run 或独立后台资源。
type Plugin struct{}

// New 创建委派工具插件。
func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "subagent-tools" }

func (p *Plugin) Start(h *host.Host) error {
	service, err := host.Resolve[*delegation.Subagents](h, "subagents")
	if err != nil {
		return err
	}
	registry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		return err
	}
	for _, tool := range toolEntries(service) {
		err = registry.Register(tool)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Plugin) Close() error { return nil }
