// Package projectjson 提供把项目保存为 JSON 文件的持久化插件。
package projectjson

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"harness/projects"
)

// Store 是一个目录里的项目库；一个项目对应一个 JSON 文件。
type Store struct {
	mu   sync.Mutex
	root string
}

var _ projects.Store = (*Store)(nil)

// New 建项目目录，之后每个项目独占一个 JSON 文件。
func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("项目目录不能为空")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析项目目录失败：%w", err)
	}
	err = os.MkdirAll(absolute, 0o700)
	if err != nil {
		return nil, fmt.Errorf("创建项目目录失败：%w", err)
	}
	err = os.Chmod(absolute, 0o700)
	if err != nil {
		return nil, fmt.Errorf("收紧项目目录权限失败：%w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("打开项目目录失败：%w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("项目路径 %s 不是目录", absolute)
	}
	return &Store{root: absolute}, nil
}

// Create 保存一个新项目；相同身份绝不覆盖。
func (s *Store) Create(project projects.Project) error {
	err := projects.Validate(project)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(project)
	if err != nil {
		return fmt.Errorf("编码项目 %s 失败：%w", project.ID, err)
	}
	path := s.path(project.ID)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("创建项目 %s 失败：%w", project.ID, err)
	}
	err = writeFull(file, data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("写项目 %s 失败：%w", project.ID, err)
	}
	return nil
}

// Get 读取一份项目副本。
func (s *Store) Get(id string) (projects.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

// List 按项目身份列出所有项目。
func (s *Store) List() ([]projects.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("读取项目目录失败：%w", err)
	}
	listed := make([]projects.Project, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		encodedID := entry.Name()[:len(entry.Name())-len(".json")]
		idData, err := base64.RawURLEncoding.DecodeString(encodedID)
		if err != nil {
			return nil, fmt.Errorf("项目文件 %s 的名字不合法：%w", entry.Name(), err)
		}
		project, err := s.read(string(idData))
		if err != nil {
			return nil, err
		}
		listed = append(listed, project)
	}
	sort.Slice(listed, func(i int, j int) bool {
		return listed[i].ID < listed[j].ID
	})
	return listed, nil
}

// Update 原子替换一个已有项目；项目身份和根目录不能改。
func (s *Store) Update(project projects.Project) error {
	err := projects.Validate(project)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	old, err := s.read(project.ID)
	if err != nil {
		return err
	}
	if old.Root != project.Root {
		return fmt.Errorf("项目 %s 的根目录创建后不能修改", project.ID)
	}
	return s.replace(project)
}

func (s *Store) read(id string) (projects.Project, error) {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return projects.Project{}, fmt.Errorf("读取项目 %s 失败：%w", id, err)
	}
	var project projects.Project
	err = json.Unmarshal(data, &project)
	if err != nil {
		return projects.Project{}, fmt.Errorf("解开项目 %s 失败：%w", id, err)
	}
	if project.ID != id {
		return projects.Project{}, fmt.Errorf("项目文件属于 %s，不是 %s", project.ID, id)
	}
	err = projects.Validate(project)
	if err != nil {
		return projects.Project{}, err
	}
	return project, nil
}

func (s *Store) replace(project projects.Project) error {
	data, err := json.Marshal(project)
	if err != nil {
		return fmt.Errorf("编码项目 %s 失败：%w", project.ID, err)
	}
	temp, err := os.CreateTemp(s.root, ".project-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	err = writeFull(temp, data)
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("写项目 %s 新内容失败：%w", project.ID, err)
	}
	err = os.Rename(tempName, s.path(project.ID))
	if err != nil {
		return fmt.Errorf("替换项目 %s 失败：%w", project.ID, err)
	}
	return nil
}

func (s *Store) path(id string) string {
	name := base64.RawURLEncoding.EncodeToString([]byte(id))
	if name == "" {
		name = "empty"
	}
	return filepath.Join(s.root, name+".json")
}

func writeFull(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
