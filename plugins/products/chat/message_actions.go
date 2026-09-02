package chat

import (
	"fmt"
	"sort"
	"strings"
)

// 活对象。messageActionRegistry 保存已填入 Chat 消息动作登记处的动作。
type messageActionRegistry struct {
	actions map[string]MessageAction
}

func newMessageActionRegistry() *messageActionRegistry {
	return &messageActionRegistry{actions: make(map[string]MessageAction)}
}

// RegisterMessageAction 填入一个消息动作。
func (r *messageActionRegistry) RegisterMessageAction(action MessageAction) error {
	if action == nil {
		return fmt.Errorf("chat: register nil message action")
	}
	definition := action.Definition()
	if strings.TrimSpace(definition.ID) == "" ||
		strings.TrimSpace(definition.Name) == "" ||
		strings.TrimSpace(definition.Icon) == "" ||
		len(definition.Targets) == 0 {
		return fmt.Errorf("chat: message action definition has empty required field")
	}
	for _, target := range definition.Targets {
		if target != MessageCardUser && target != MessageCardAssistant {
			return fmt.Errorf("chat: message action %q has unknown target %q", definition.ID, target)
		}
	}
	if _, exists := r.actions[definition.ID]; exists {
		return fmt.Errorf("chat: message action %q already registered", definition.ID)
	}
	r.actions[definition.ID] = action
	return nil
}

// MessageActions 返回稳定排序后的动作定义。
func (r *messageActionRegistry) MessageActions() []MessageActionDefinition {
	definitions := make([]MessageActionDefinition, 0, len(r.actions))
	for _, action := range r.actions {
		definitions = append(definitions, action.Definition())
	}
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].Order == definitions[j].Order {
			return definitions[i].ID < definitions[j].ID
		}
		return definitions[i].Order < definitions[j].Order
	})
	return definitions
}

// MessageAction 按 ID 返回已登记的动作。
func (r *messageActionRegistry) MessageAction(id string) (MessageAction, bool) {
	action, ok := r.actions[id]
	return action, ok
}
