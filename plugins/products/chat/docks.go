package chat

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var dockIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// 活对象。DockRegistry 保存已填入 Chat 输入框上方 Dock 插槽的条目。
type DockRegistry struct {
	mu    sync.RWMutex
	docks map[string]Dock
}

func newDockRegistry() *DockRegistry {
	return &DockRegistry{docks: make(map[string]Dock)}
}

// RegisterDock 填入一个持续状态条目。
func (r *DockRegistry) RegisterDock(dock Dock) error {
	if dock == nil {
		return fmt.Errorf("chat: register nil dock")
	}
	definition := dock.Definition()
	if !dockIDPattern.MatchString(definition.ID) {
		return fmt.Errorf("chat: dock ID %q is invalid", definition.ID)
	}
	if strings.TrimSpace(definition.Name) == "" {
		return fmt.Errorf("chat: dock definition has empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.docks[definition.ID]; exists {
		return fmt.Errorf("chat: dock %q already registered", definition.ID)
	}
	r.docks[definition.ID] = dock
	return nil
}

// Docks 返回稳定排序后的 Dock 定义。
func (r *DockRegistry) Docks() []DockDefinition {
	r.mu.RLock()
	definitions := make([]DockDefinition, 0, len(r.docks))
	for _, dock := range r.docks {
		definitions = append(definitions, dock.Definition())
	}
	r.mu.RUnlock()
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].Order == definitions[j].Order {
			return definitions[i].ID < definitions[j].ID
		}
		return definitions[i].Order < definitions[j].Order
	})
	return definitions
}

// Dock 按 ID 返回已登记的 Dock 条目。
func (r *DockRegistry) Dock(id string) (Dock, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	dock, ok := r.docks[id]
	return dock, ok
}
