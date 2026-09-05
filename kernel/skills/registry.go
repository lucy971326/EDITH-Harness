package skills

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// 活对象。挂在 Host 的 skills 键上的 Skill Provider 登记处。
type Registry struct {
	mu        sync.RWMutex
	providers []providerEntry
	names     map[string]struct{}
}

// 数据。一条已登记 Provider 及其稳定身份。
type providerEntry struct {
	name     string
	provider Provider
}

// NewRegistry 造一张空的 Skill Provider 登记处。
func NewRegistry() *Registry {
	return &Registry{names: make(map[string]struct{})}
}

// Register 填入一个启动时固定的 Skill Provider。
func (r *Registry) Register(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("skills: register nil provider")
	}
	name := strings.TrimSpace(provider.Name())
	if name == "" {
		return fmt.Errorf("skills: register provider with empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.names[name]; exists {
		return fmt.Errorf("skills: provider %q already registered", name)
	}
	r.providers = append(r.providers, providerEntry{name: name, provider: provider})
	r.names[name] = struct{}{}
	return nil
}

// List 按名称稳定合并所有 Provider 在工作区发现的 Skill。
func (r *Registry) List(workspace string) ([]Skill, error) {
	r.mu.RLock()
	providers := append([]providerEntry(nil), r.providers...)
	r.mu.RUnlock()

	all := make([]Skill, 0)
	owners := make(map[string]string)
	for _, entry := range providers {
		provided, err := entry.provider.List(workspace)
		if err != nil {
			return nil, fmt.Errorf("skills: provider %q: %w", entry.name, err)
		}
		seen := make(map[string]struct{}, len(provided))
		for _, skill := range provided {
			skill, err = normalizeSkill(skill)
			if err != nil {
				return nil, fmt.Errorf("skills: provider %q: %w", entry.name, err)
			}
			if _, duplicate := seen[skill.Name]; duplicate {
				return nil, fmt.Errorf("skills: provider %q returned duplicate %q", entry.name, skill.Name)
			}
			seen[skill.Name] = struct{}{}
			if previous, duplicate := owners[skill.Name]; duplicate {
				return nil, fmt.Errorf("skills: %q is provided by providers %q and %q", skill.Name, previous, entry.name)
			}
			owners[skill.Name] = entry.name
			all = append(all, skill)
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Name < all[j].Name
	})
	return all, nil
}

func normalizeSkill(skill Skill) (Skill, error) {
	skill.Name = strings.TrimSpace(skill.Name)
	skill.Description = strings.TrimSpace(skill.Description)
	skill.Location = strings.TrimSpace(skill.Location)
	if skill.Name == "" {
		return Skill{}, fmt.Errorf("register empty name")
	}
	if skill.Description == "" {
		return Skill{}, fmt.Errorf("register %q with empty description", skill.Name)
	}
	if skill.Location == "" {
		return Skill{}, fmt.Errorf("register %q with empty location", skill.Name)
	}
	if skill.Scope != ScopeSystem && skill.Scope != ScopeUser && skill.Scope != ScopeWorkspace {
		return Skill{}, fmt.Errorf("register %q with invalid scope %q", skill.Name, skill.Scope)
	}
	return skill, nil
}
