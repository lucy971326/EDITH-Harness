package chat

import (
	"fmt"
	"sort"
	"strings"
)

// 活对象。PanelRegistry 保存已填入 Chat 右侧面板登记处的面板类型。
type PanelRegistry struct {
	panels map[string]Panel
}

func newPanelRegistry() *PanelRegistry {
	return &PanelRegistry{panels: make(map[string]Panel)}
}

// RegisterPanel 填入一个面板类型。
func (r *PanelRegistry) RegisterPanel(panel Panel) error {
	if panel == nil {
		return fmt.Errorf("chat: register nil panel")
	}
	definition := panel.Definition()
	if strings.TrimSpace(definition.ID) == "" ||
		strings.TrimSpace(definition.Name) == "" ||
		strings.TrimSpace(definition.Icon) == "" ||
		strings.TrimSpace(definition.DefaultInstanceKey) == "" ||
		strings.TrimSpace(definition.DefaultTabTitle) == "" {
		return fmt.Errorf("chat: panel definition has empty required field")
	}
	if _, exists := r.panels[definition.ID]; exists {
		return fmt.Errorf("chat: panel %q already registered", definition.ID)
	}
	r.panels[definition.ID] = panel
	return nil
}

// Panels 返回稳定排序后的面板定义。
func (r *PanelRegistry) Panels() []PanelDefinition {
	definitions := make([]PanelDefinition, 0, len(r.panels))
	for _, panel := range r.panels {
		definitions = append(definitions, panel.Definition())
	}
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].Order == definitions[j].Order {
			return definitions[i].ID < definitions[j].ID
		}
		return definitions[i].Order < definitions[j].Order
	})
	return definitions
}

// Panel 按 ID 返回已登记的面板类型。
func (r *PanelRegistry) Panel(id string) (Panel, bool) {
	panel, ok := r.panels[id]
	return panel, ok
}
