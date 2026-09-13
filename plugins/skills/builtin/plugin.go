// Package builtin 提供 Harness 自带的 Skill。
package builtin

import (
	_ "embed"
	"fmt"

	"harness/kernel/host"
	"harness/kernel/machine"
	kernskills "harness/kernel/skills"
)

//go:embed assets/skill-creator/SKILL.md
var skillCreator []byte

// 活对象。向 skills 登记 Harness 内置 Skill 的插件。
type Plugin struct {
	provider *Provider
}

// New 造 Harness 内置 Skill Provider 插件。
func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "skills-builtin" }

func (p *Plugin) Start(h *host.Host) error {
	registry, err := host.Resolve[kernskills.Skills](h, "skills")
	if err != nil {
		return fmt.Errorf("skills-builtin: resolve skills: %w", err)
	}
	machineService, err := host.Resolve[machine.Machine](h, "machine")
	if err != nil {
		return fmt.Errorf("skills-builtin: resolve machine: %w", err)
	}
	p.provider, err = newProvider(machineService, skillCreator)
	if err != nil {
		return err
	}
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
