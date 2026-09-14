// Package applypatch 提供 Codex 风格的结构化文件修改 Tool。
package applypatch

import (
	"harness/kernel/host"
	"harness/kernel/machine"
	"harness/kernel/tools"
)

// 活对象。启动时填入 apply_patch Tool。
type Plugin struct{}

// New 造 apply_patch Tool 插件。
func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "apply-patch" }

func (p *Plugin) Start(h *host.Host) error {
	files, err := host.Resolve[machine.FileSystem](h, "machine")
	if err != nil {
		return err
	}
	registry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		return err
	}
	err = registry.Register(newTool(files))
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) Close() error { return nil }
