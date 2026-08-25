package projects

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"harness/session"
)

// HeaderStore 列出已保存会话的封面，供项目查询自己的历史会话。
type HeaderStore interface {
	ListHeaders() ([]session.Header, error)
}

// Service 提供项目登记和项目会话查询。
type Service interface {
	Create(name string, root string) (Project, error)
	Get(id string) (Project, error)
	List() ([]Project, error)
	Rename(id string, name string) error
	Archive(id string) error
	Restore(id string) error
	RememberPreset(id string, presetID string) error
	ListSessions(projectID string) ([]session.Header, error)
}

// service 组合项目存储和会话封面来源。
type service struct {
	mu       sync.Mutex
	store    Store
	sessions HeaderStore
}

// New 组合项目存储和会话封面来源，返回项目服务。
func New(store Store, sessions HeaderStore) Service {
	return &service{
		store:    store,
		sessions: sessions,
	}
}

// Create 按名称和目录登记项目，返回自动生成身份且根目录已规范化的项目。
func (s *service) Create(name string, root string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, fmt.Errorf("项目名称不能为空")
	}
	normalized, err := normalizeRoot(root)
	if err != nil {
		return Project{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	listed, err := s.store.List()
	if err != nil {
		return Project{}, fmt.Errorf("列出已有项目失败：%w", err)
	}
	for _, existing := range listed {
		if existing.Root == normalized {
			return Project{}, fmt.Errorf("目录 %s 已登记为项目 %s", normalized, existing.ID)
		}
	}

	id, err := availableID(listed)
	if err != nil {
		return Project{}, err
	}
	project := Project{
		ID:   id,
		Name: name,
		Root: normalized,
	}
	err = s.store.Create(project)
	if err != nil {
		return Project{}, fmt.Errorf("创建项目 %s 失败：%w", project.ID, err)
	}
	return project, nil
}

// Get 按稳定身份读取一个项目。
func (s *service) Get(id string) (Project, error) {
	project, err := s.store.Get(id)
	if err != nil {
		return Project{}, err
	}
	return project, nil
}

// List 按项目身份列出所有项目，包括归档项目。
func (s *service) List() ([]Project, error) {
	listed, err := s.store.List()
	if err != nil {
		return nil, err
	}
	sort.Slice(listed, func(i int, j int) bool { return listed[i].ID < listed[j].ID })
	return listed, nil
}

// Rename 修改一个项目给用户看的名称。
func (s *service) Rename(id string, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("项目名称不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	project, err := s.store.Get(id)
	if err != nil {
		return err
	}
	project.Name = name
	err = s.store.Update(project)
	if err != nil {
		return fmt.Errorf("重命名项目 %s 失败：%w", id, err)
	}
	return nil
}

// Archive 标记项目为归档；工作目录和历史会话都保留。
func (s *service) Archive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if project.Archived {
		return nil
	}
	project.Archived = true
	err = s.store.Update(project)
	if err != nil {
		return fmt.Errorf("归档项目 %s 失败：%w", id, err)
	}
	return nil
}

// Restore 取消项目归档，恢复其可用状态。
func (s *service) Restore(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, err := s.store.Get(id)
	if err != nil {
		return err
	}
	if !project.Archived {
		return nil
	}
	project.Archived = false
	err = s.store.Update(project)
	if err != nil {
		return fmt.Errorf("恢复项目 %s 失败：%w", id, err)
	}
	return nil
}

// RememberPreset 记住项目上次成功使用的 Agent 模式。
func (s *service) RememberPreset(id string, presetID string) error {
	if presetID == "" {
		return fmt.Errorf("Agent 模式 id 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	project, err := s.store.Get(id)
	if err != nil {
		return err
	}
	project.LastPresetID = presetID
	err = s.store.Update(project)
	if err != nil {
		return fmt.Errorf("记住项目 %s 的 Agent 模式失败：%w", id, err)
	}
	return nil
}

// ListSessions 列出属于一个项目的全部会话封面，不打开会话账本。
func (s *service) ListSessions(projectID string) ([]session.Header, error) {
	_, err := s.store.Get(projectID)
	if err != nil {
		return nil, err
	}
	headers, err := s.sessions.ListHeaders()
	if err != nil {
		return nil, fmt.Errorf("列出项目 %s 的会话失败：%w", projectID, err)
	}
	matched := make([]session.Header, 0)
	for _, header := range headers {
		if header.ProjectID == projectID {
			matched = append(matched, header)
		}
	}
	return matched, nil
}

func normalizeRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("项目根目录不能为空")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("解析项目根目录失败：%w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("解析项目根目录的符号链接失败：%w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("打开项目根目录失败：%w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("项目根目录 %s 不是目录", resolved)
	}
	return resolved, nil
}

func availableID(listed []Project) (string, error) {
	known := make(map[string]bool, len(listed))
	for _, project := range listed {
		known[project.ID] = true
	}
	for range 10 {
		id, err := newID()
		if err != nil {
			return "", err
		}
		if !known[id] {
			return id, nil
		}
	}
	return "", fmt.Errorf("连续生成 10 次项目 id 都已存在")
}

func newID() (string, error) {
	data := make([]byte, 16)
	_, err := rand.Read(data)
	if err != nil {
		return "", fmt.Errorf("生成项目 id 失败：%w", err)
	}
	return hex.EncodeToString(data), nil
}
