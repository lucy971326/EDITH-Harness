// Package filesystem 从 Harness 和 Agent Skills 约定的目录发现 Skill。
package filesystem

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"harness/kernel/machine"
	"harness/kernel/persist"
	kernskills "harness/kernel/skills"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// 活对象。使用 machine 扫描本机 Skill 根目录的 Provider。
type Provider struct {
	machine machine.Machine
	files   *persist.Files
}

// newProvider 造一个文件系统 Skill Provider。
func newProvider(m machine.Machine, files *persist.Files) *Provider {
	return &Provider{machine: m, files: files}
}

// Name 返回此 Provider 的稳定名称。
func (p *Provider) Name() string { return "filesystem" }

// List 按项目优先、同层 .harness 优先的顺序发现 Skill。
func (p *Provider) List(workspace string) ([]kernskills.Skill, error) {
	if p == nil || p.machine == nil || p.files == nil {
		return nil, fmt.Errorf("skills-filesystem: missing dependency")
	}
	home, err := p.machine.HomeDir()
	if err != nil {
		return nil, fmt.Errorf("skills-filesystem: home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return nil, fmt.Errorf("skills-filesystem: home directory is empty")
	}

	roots := make([]root, 0, 4)
	if workspace != "" {
		roots = append(roots,
			root{path: p.machine.ResolvePath(workspace, ".harness/skills"), scope: kernskills.ScopeWorkspace},
			root{path: p.machine.ResolvePath(workspace, ".agents/skills"), scope: kernskills.ScopeWorkspace},
		)
	}
	userSkills, err := p.files.Scope("skills")
	if err != nil {
		return nil, err
	}
	roots = append(roots,
		root{files: userSkills, scope: kernskills.ScopeUser},
		root{path: p.machine.ResolvePath(home, ".agents/skills"), scope: kernskills.ScopeUser},
	)

	found := make(map[string]kernskills.Skill)
	for _, candidateRoot := range roots {
		entries, err := p.readRoot(candidateRoot)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("skills-filesystem: read root %q: %w", candidateRoot.path, err)
		}
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Name < entries[j].Name
		})
		for _, entry := range entries {
			if !entry.IsDir {
				continue
			}
			skill, ok, err := p.readSkill(candidateRoot, entry.Name)
			if err != nil {
				return nil, fmt.Errorf("skills-filesystem: skill %q: %w", entry.Name, err)
			}
			if !ok {
				continue
			}
			if _, exists := found[skill.Name]; exists {
				continue
			}
			found[skill.Name] = skill
		}
	}

	out := make([]kernskills.Skill, 0, len(found))
	for _, skill := range found {
		out = append(out, skill)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// 数据。一个待扫描的 Skill 根及其作用域。
type root struct {
	path  string
	files *persist.Files
	scope kernskills.Scope
}

type rootEntry struct {
	Name  string
	IsDir bool
}

func (p *Provider) readRoot(candidate root) ([]rootEntry, error) {
	if candidate.files != nil {
		entries, err := candidate.files.List()
		if err != nil {
			return nil, err
		}
		out := make([]rootEntry, 0, len(entries))
		for _, entry := range entries {
			out = append(out, rootEntry{Name: entry.Name, IsDir: entry.IsDir})
		}
		return out, nil
	}

	entries, err := p.machine.ReadDir(candidate.path)
	if err != nil {
		return nil, err
	}
	out := make([]rootEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, rootEntry{Name: entry.Name, IsDir: entry.IsDir})
	}
	return out, nil
}

func (p *Provider) readSkill(candidate root, directoryName string) (kernskills.Skill, bool, error) {
	var path string
	var data []byte
	var err error
	if candidate.files != nil {
		skillFiles, scopeErr := candidate.files.Scope(directoryName)
		if scopeErr != nil {
			return kernskills.Skill{}, false, scopeErr
		}
		data, err = skillFiles.Read("SKILL.md")
		path, scopeErr = skillFiles.Path("SKILL.md")
		if scopeErr != nil {
			return kernskills.Skill{}, false, scopeErr
		}
	} else {
		skillDir := p.machine.ResolvePath(candidate.path, directoryName)
		path = p.machine.ResolvePath(skillDir, "SKILL.md")
		data, err = p.machine.ReadFile(path)
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return kernskills.Skill{}, false, nil
		}
		return kernskills.Skill{}, false, fmt.Errorf("read SKILL.md: %w", err)
	}

	name, description, err := parseFrontmatter(data)
	if err != nil {
		return kernskills.Skill{}, false, err
	}
	if err := validateName(name); err != nil {
		return kernskills.Skill{}, false, err
	}
	if name != directoryName {
		return kernskills.Skill{}, false, fmt.Errorf("frontmatter name %q does not match directory %q", name, directoryName)
	}
	if err := validateDescription(description); err != nil {
		return kernskills.Skill{}, false, err
	}
	return kernskills.Skill{
		Name:        name,
		Description: description,
		Location:    path,
		Scope:       candidate.scope,
	}, true, nil
}

func parseFrontmatter(data []byte) (string, string, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return "", "", fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}

	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSuffix(lines[index], "\r") == "---" {
			end = index
			break
		}
	}
	if end < 0 {
		return "", "", fmt.Errorf("SKILL.md frontmatter is not closed")
	}

	var frontmatter struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &frontmatter)
	if err != nil {
		return "", "", fmt.Errorf("invalid frontmatter: %w", err)
	}
	return frontmatter.Name, strings.TrimSpace(frontmatter.Description), nil
}

func validateName(name string) error {
	length := utf8.RuneCountInString(name)
	if length < 1 || length > 64 {
		return fmt.Errorf("frontmatter name must be 1-64 characters")
	}
	if !skillNamePattern.MatchString(name) {
		return fmt.Errorf("frontmatter name %q is not a valid Agent Skill name", name)
	}
	if strings.Contains(name, "--") {
		return fmt.Errorf("frontmatter name %q must not contain consecutive hyphens", name)
	}
	return nil
}

func validateDescription(description string) error {
	length := utf8.RuneCountInString(description)
	if length < 1 || length > 1024 {
		return fmt.Errorf("frontmatter description must be 1-1024 characters")
	}
	return nil
}
