package skills

import (
	"fmt"
	"sort"
	"sync"
)

// 活对象。挂在 Host 的 skills 键上的 Skill 摘要登记处。
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Skill
}

// NewRegistry 造一张空的 Skill 摘要登记处。
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]Skill)}
}

// Register 填入一个启动时固定的 Skill 摘要。
func (r *Registry) Register(skill Skill) error {
	if skill.Name == "" {
		return fmt.Errorf("skills: register empty name")
	}
	if skill.Summary == "" {
		return fmt.Errorf("skills: register %q with empty summary", skill.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[skill.Name]; exists {
		return fmt.Errorf("skills: %q already registered", skill.Name)
	}
	r.entries[skill.Name] = skill
	return nil
}

// Get 按 names 的顺序取出 Skill；缺失或重复名称都报错。
func (r *Registry) Get(names []string) ([]Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Skill, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("skills: %q appears more than once", name)
		}
		seen[name] = struct{}{}
		skill, ok := r.entries[name]
		if !ok {
			return nil, fmt.Errorf("skills: %q is not registered", name)
		}
		out = append(out, skill)
	}
	return out, nil
}

// List 按名称稳定列出当前全部 Skill。
func (r *Registry) List() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Skill, 0, len(r.entries))
	for _, skill := range r.entries {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
