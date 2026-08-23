package profilejson

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

	"harness/agents"
)

// Store 是一个目录里的 Agent 档案库；一档一文件，更新用替换保证旧档不被写坏。
type Store struct {
	mu   sync.Mutex
	root string
}

var _ agents.ProfileStore = (*Store)(nil)

// New 建档案目录，之后每个 Agent 独占一个 JSON 文件。
func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("Agent 档案目录不能为空")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析 Agent 档案目录失败：%w", err)
	}
	err = os.MkdirAll(absolute, 0o755)
	if err != nil {
		return nil, fmt.Errorf("创建 Agent 档案目录失败：%w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("打开 Agent 档案目录失败：%w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Agent 档案路径 %s 不是目录", absolute)
	}
	return &Store{root: absolute}, nil
}

// Create 保存一个新档案；同名档案绝不覆盖。
func (s *Store) Create(profile agents.AgentProfile) error {
	if err := validateNew(profile); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("编码 agent %s 档案失败：%w", profile.ID, err)
	}
	path := s.path(profile.ID)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("创建 agent %s 档案失败：%w", profile.ID, err)
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
		return fmt.Errorf("写 agent %s 档案失败：%w", profile.ID, err)
	}
	return nil
}

// Update 原子替换一个已有档案；档案不存在或版本没有递增都报错。
func (s *Store) Update(profile agents.AgentProfile) error {
	if err := validateNew(profile); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	old, err := s.read(profile.ID)
	if err != nil {
		return err
	}
	if profile.Revision != old.Revision+1 {
		return fmt.Errorf("agent %s 档案版本要从 %d 升到 %d", profile.ID, old.Revision, old.Revision+1)
	}
	if old.Archived != profile.Archived {
		return fmt.Errorf("agent %s 的归档状态只许 Archive 改", profile.ID)
	}
	return s.replace(profile)
}

// Get 读取一份档案副本。
func (s *Store) Get(id string) (agents.AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

// List 按 id 列出所有档案，方便 UI 或命令行调用。
func (s *Store) List() ([]agents.AgentProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	profiles := make([]agents.AgentProfile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		idData, err := base64.RawURLEncoding.DecodeString(entry.Name()[:len(entry.Name())-len(".json")])
		if err != nil {
			return nil, fmt.Errorf("档案文件 %s 的名字不合法：%w", entry.Name(), err)
		}
		profile, err := s.read(string(idData))
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i int, j int) bool { return profiles[i].ID < profiles[j].ID })
	return profiles, nil
}

// Archive 把档案标成归档；历史会话仍可凭它恢复。
func (s *Store) Archive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profile, err := s.read(id)
	if err != nil {
		return err
	}
	if profile.Archived {
		return nil
	}
	profile.Archived = true
	profile.Revision++
	return s.replace(profile)
}

func (s *Store) read(id string) (agents.AgentProfile, error) {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return agents.AgentProfile{}, fmt.Errorf("读取 agent %s 档案失败：%w", id, err)
	}
	var profile agents.AgentProfile
	err = json.Unmarshal(data, &profile)
	if err != nil {
		return agents.AgentProfile{}, fmt.Errorf("解开 agent %s 档案失败：%w", id, err)
	}
	if profile.ID != id {
		return agents.AgentProfile{}, fmt.Errorf("档案文件属于 agent %s，不是 %s", profile.ID, id)
	}
	if err := validateNew(profile); err != nil {
		return agents.AgentProfile{}, err
	}
	return profile, nil
}

func (s *Store) replace(profile agents.AgentProfile) error {
	data, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("编码 agent %s 档案失败：%w", profile.ID, err)
	}
	temp, err := os.CreateTemp(s.root, ".agent-profile-*")
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
		return fmt.Errorf("写 agent %s 新档案失败：%w", profile.ID, err)
	}
	err = os.Rename(tempName, s.path(profile.ID))
	if err != nil {
		return fmt.Errorf("替换 agent %s 档案失败：%w", profile.ID, err)
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

func validateNew(profile agents.AgentProfile) error { return agents.ValidateProfile(profile) }

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
