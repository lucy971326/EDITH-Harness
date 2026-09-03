package chat

import (
	"fmt"
	"regexp"
	"sort"
	"sync"
)

var composerActionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// 活对象。composerActionRegistry 保存已填入 Chat 输入工具栏插槽的动作。
type composerActionRegistry struct {
	mu      sync.RWMutex
	actions map[string]ComposerAction
}

func newComposerActionRegistry() *composerActionRegistry {
	return &composerActionRegistry{actions: make(map[string]ComposerAction)}
}

// RegisterComposerAction 填入一个输入工具栏动作。
func (r *composerActionRegistry) RegisterComposerAction(action ComposerAction) error {
	if action == nil {
		return fmt.Errorf("chat: register nil composer action")
	}
	definition := action.Definition()
	if !composerActionIDPattern.MatchString(definition.ID) {
		return fmt.Errorf("chat: composer action ID %q is invalid", definition.ID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.actions[definition.ID]; exists {
		return fmt.Errorf("chat: composer action %q already registered", definition.ID)
	}
	r.actions[definition.ID] = action
	return nil
}

// ComposerActions 返回稳定排序后的输入动作定义。
func (r *composerActionRegistry) ComposerActions() []ComposerActionDefinition {
	r.mu.RLock()
	definitions := make([]ComposerActionDefinition, 0, len(r.actions))
	for _, action := range r.actions {
		definitions = append(definitions, action.Definition())
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

// ComposerAction 按 ID 返回已登记的输入动作。
func (r *composerActionRegistry) ComposerAction(id string) (ComposerAction, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	action, ok := r.actions[id]
	return action, ok
}
