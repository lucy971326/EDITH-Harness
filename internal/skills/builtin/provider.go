package builtin

import (
	_ "embed"
	"fmt"
	"harness/internal/persist"

	kernskills "harness/internal/skills"
)

// 活对象。把内置 Skill 物化到当前用户目录的 Provider。
type Provider struct {
	location string
}

func newProvider(files *persist.Files, content []byte) (*Provider, error) {
	if files == nil {
		return nil, fmt.Errorf("skills-builtin: nil persist")
	}
	skillFiles, err := files.Scope("system", "skills", "skill-creator")
	if err != nil {
		return nil, err
	}
	err = skillFiles.Write("SKILL.md", content)
	if err != nil {
		return nil, fmt.Errorf("skills-builtin: materialize skill-creator: %w", err)
	}
	location, err := skillFiles.Path("SKILL.md")
	if err != nil {
		return nil, err
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

//go:embed assets/skill-creator/SKILL.md
var skillCreator []byte

// New 创建内置 Skill 来源。
func New(files *persist.Files) (*Provider, error) { return newProvider(files, skillCreator) }
