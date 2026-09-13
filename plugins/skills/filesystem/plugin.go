// Package filesystem 提供文件系统 Skill Provider 插件。
package filesystem

import (
	"fmt"

	"harness/kernel/host"
	"harness/kernel/machine"
	kernskills "harness/kernel/skills"
)

// 活对象。向 skills 登记文件系统 Provider 的插件。
type Plugin struct {
	provider *Provider
}

// New 造文件系统 Skill Provider 插件。
func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "skills-filesystem" }

func (p *Plugin) Start(h *host.Host) error {
	registry, err := host.Resolve[kernskills.Skills](h, "skills")
	if err != nil {
		return fmt.Errorf("skills-filesystem: resolve skills: %w", err)
	}
	machineService, err := host.Resolve[machine.Machine](h, "machine")
	if err != nil {
		return fmt.Errorf("skills-filesystem: resolve machine: %w", err)
	}
	p.provider = newProvider(machineService)
	if err := registry.Register(p.provider); err != nil {
		p.provider = nil
		return err
	}
	return nil
}

func (p *Plugin) Close() error {
	p.provider = nil
	return nil
}
