package builtin

import (
	"fmt"

	"harness/kernel/machine"
	kernskills "harness/kernel/skills"
)

const skillCreatorLocation = ".harness/system-skills/skill-creator/SKILL.md"

// 活对象。把内置 Skill 物化到当前用户目录的 Provider。
type Provider struct {
	location string
}

func newProvider(m machine.Machine, content []byte) (*Provider, error) {
	if m == nil {
		return nil, fmt.Errorf("skills-builtin: nil machine")
	}
	home, err := m.HomeDir()
	if err != nil {
		return nil, fmt.Errorf("skills-builtin: home directory: %w", err)
	}
	if home == "" {
		return nil, fmt.Errorf("skills-builtin: home directory is empty")
	}
	location := m.ResolvePath(home, skillCreatorLocation)
	if err := m.WriteFile(location, content); err != nil {
		return nil, fmt.Errorf("skills-builtin: materialize skill-creator: %w", err)
	}
	return &Provider{location: location}, nil
}

func (p *Provider) Name() string { return "builtin" }

func (p *Provider) List(string) ([]kernskills.Skill, error) {
	if p == nil || p.location == "" {
		return nil, fmt.Errorf("skills-builtin: provider is not ready")
	}
	return []kernskills.Skill{
		{
			Name:        "skill-creator",
			Description: "创建或修改 Harness Skill。",
			Location:    p.location,
			Scope:       kernskills.ScopeSystem,
		},
	}, nil
}
