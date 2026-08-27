package config

import (
	"fmt"
	"os"
	"sync"

	"go.yaml.in/yaml/v3"
)

var _ Settings = (*settings)(nil)

type settings struct {
	mu      sync.Mutex
	path    string
	drawers map[string]*Drawer
	user    map[string]map[string]string
}

func newSettings(path string, user map[string]map[string]string) *settings {
	if user == nil {
		user = map[string]map[string]string{}
	}
	return &settings{
		path:    path,
		drawers: map[string]*Drawer{},
		user:    user,
	}
}

// Register 登记一格抽屉；同名重复是组装错误。
func (s *settings) Register(drawer Drawer) (func(), error) {
	normalized, err := normalizeDrawer(drawer)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, taken := s.drawers[normalized.Name]
	if taken {
		return nil, fmt.Errorf("抽屉 %s 已登记", normalized.Name)
	}
	merged := s.mergeLocked(normalized)
	if normalized.Validate != nil {
		err = normalized.Validate(merged)
		if err != nil {
			return nil, err
		}
	}
	entry := &Drawer{}
	*entry = normalized
	s.drawers[normalized.Name] = entry
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		current, exists := s.drawers[normalized.Name]
		if !exists {
			return
		}
		if current != entry {
			return
		}
		delete(s.drawers, normalized.Name)
	}, nil
}

// Get 返回一格合并后的配置：默认值再盖上文件里的值。
func (s *settings) Get(name string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	drawer, exists := s.drawers[name]
	if !exists {
		return nil, fmt.Errorf("抽屉 %s 未登记", name)
	}
	return s.mergeLocked(*drawer), nil
}

// Set 写入一格用户配置并落盘；未登记的抽屉不能写。
func (s *settings) Set(name string, values map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	drawer, exists := s.drawers[name]
	if !exists {
		return fmt.Errorf("抽屉 %s 未登记", name)
	}
	candidate := cloneTable(s.user)
	candidate[name] = cloneMap(values)
	merged := mergeDrawer(*drawer, candidate)
	if drawer.Validate != nil {
		err := drawer.Validate(merged)
		if err != nil {
			return err
		}
	}
	err := s.persistLocked(candidate)
	if err != nil {
		return err
	}
	s.user = candidate
	return nil
}

func (s *settings) mergeLocked(drawer Drawer) map[string]string {
	return mergeDrawer(drawer, s.user)
}

func mergeDrawer(drawer Drawer, user map[string]map[string]string) map[string]string {
	merged := cloneMap(drawer.Defaults)
	for key, value := range user[drawer.Name] {
		if value == "" {
			continue
		}
		merged[key] = value
	}
	return merged
}

func (s *settings) persistLocked(user map[string]map[string]string) error {
	if s.path == "" {
		return nil
	}
	root := map[string]any{}
	data, err := os.ReadFile(s.path)
	if err == nil {
		err = yaml.Unmarshal(data, &root)
		if err != nil {
			return fmt.Errorf("读配置失败：%w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读配置失败：%w", err)
	}
	root["settings"] = cloneTable(user)
	out, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("写配置失败：%w", err)
	}
	err = os.WriteFile(s.path, out, 0o600)
	if err != nil {
		return fmt.Errorf("写配置失败：%w", err)
	}
	return nil
}
