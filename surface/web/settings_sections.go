package web

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var settingsSectionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// 活对象。settingsSectionRegistry 保存已填入 Web 公共设置页的插件栏目。
type settingsSectionRegistry struct {
	mu       sync.RWMutex
	sections map[string]SettingsSection
}

func newSettingsSectionRegistry() *settingsSectionRegistry {
	return &settingsSectionRegistry{sections: make(map[string]SettingsSection)}
}

// RegisterSettingsSection 填入一个设置栏目。
func (r *settingsSectionRegistry) RegisterSettingsSection(section SettingsSection) error {
	if section == nil {
		return fmt.Errorf("web: register nil settings section")
	}
	definition := section.Definition()
	if !settingsSectionIDPattern.MatchString(definition.ID) {
		return fmt.Errorf("web: settings section ID %q is invalid", definition.ID)
	}
	if definition.ID == appearanceSectionID {
		return fmt.Errorf("web: settings section ID %q is reserved", definition.ID)
	}
	if strings.TrimSpace(definition.Title) == "" {
		return fmt.Errorf("web: settings section definition has empty title")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sections[definition.ID]; exists {
		return fmt.Errorf("web: settings section %q already registered", definition.ID)
	}
	r.sections[definition.ID] = section
	return nil
}

// SettingsSections 返回稳定排序后的设置栏目定义。
func (r *settingsSectionRegistry) SettingsSections() []SettingsSectionDefinition {
	r.mu.RLock()
	definitions := make([]SettingsSectionDefinition, 0, len(r.sections))
	for _, section := range r.sections {
		definitions = append(definitions, section.Definition())
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

// SettingsSection 按 ID 返回已登记的设置栏目。
func (r *settingsSectionRegistry) SettingsSection(id string) (SettingsSection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	section, ok := r.sections[id]
	return section, ok
}
