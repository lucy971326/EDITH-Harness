package presetjson

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"harness/presets"
)

// Store 是一个目录里的 Agent 模式版本库；每个模式一个目录，每个版本一个 JSON 文件。
type Store struct {
	mu   sync.Mutex
	root string
}

var _ presets.Store = (*Store)(nil)

// New 建模式库目录，之后每个模式的历史版本独占一个目录。
func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("Agent 模式目录不能为空")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析 Agent 模式目录失败：%w", err)
	}
	err = os.MkdirAll(absolute, 0o700)
	if err != nil {
		return nil, fmt.Errorf("创建 Agent 模式目录失败：%w", err)
	}
	err = os.Chmod(absolute, 0o700)
	if err != nil {
		return nil, fmt.Errorf("收紧 Agent 模式目录权限失败：%w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("打开 Agent 模式目录失败：%w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Agent 模式路径 %s 不是目录", absolute)
	}
	return &Store{root: absolute}, nil
}

// Create 保存第 1 个模式版本；同名模式绝不覆盖。
func (s *Store) Create(revision presets.Revision) error {
	if revision.Revision != 1 {
		return fmt.Errorf("Agent 模式 %s 必须从版本 1 创建", revision.ID)
	}
	if revision.Archived {
		return fmt.Errorf("Agent 模式 %s 不能直接创建为归档状态", revision.ID)
	}
	err := presets.Validate(revision)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	directory := s.directory(revision.ID)
	err = os.Mkdir(directory, 0o700)
	if err != nil {
		return fmt.Errorf("创建 Agent 模式 %s 目录失败：%w", revision.ID, err)
	}
	err = os.Chmod(directory, 0o700)
	if err != nil {
		_ = os.Remove(directory)
		return fmt.Errorf("收紧 Agent 模式 %s 目录权限失败：%w", revision.ID, err)
	}
	err = s.writeRevision(revision)
	if err != nil {
		_ = os.Remove(directory)
		return err
	}
	return nil
}

// Update 追加当前模式的后续版本；版本必须恰好接在当前版本之后。
func (s *Store) Update(revision presets.Revision) error {
	err := presets.Validate(revision)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.readCurrent(revision.ID)
	if err != nil {
		return err
	}
	if revision.Revision != current.Revision+1 {
		return fmt.Errorf("Agent 模式 %s 版本要从 %d 升到 %d", revision.ID, current.Revision, current.Revision+1)
	}
	if revision.Archived != current.Archived {
		return fmt.Errorf("Agent 模式 %s 的归档状态只许 Archive 改", revision.ID)
	}
	return s.writeRevision(revision)
}

// Get 读取一个模式的当前版本副本。
func (s *Store) Get(id string) (presets.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readCurrent(id)
}

// GetRevision 读取一个模式的指定历史版本副本。
func (s *Store) GetRevision(id string, number int) (presets.Revision, error) {
	if number < 1 {
		return presets.Revision{}, fmt.Errorf("Agent 模式 %s 的版本必须从 1 起", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readRevision(id, number)
}

// List 按 id 列出每个模式的当前版本副本。
func (s *Store) List() ([]presets.Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("列出 Agent 模式目录失败：%w", err)
	}
	listed := make([]presets.Revision, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := decodeID(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("Agent 模式目录 %s 的名字不合法：%w", entry.Name(), err)
		}
		revision, err := s.readCurrent(id)
		if err != nil {
			return nil, err
		}
		listed = append(listed, revision)
	}
	sort.Slice(listed, func(i int, j int) bool {
		return listed[i].ID < listed[j].ID
	})
	return listed, nil
}

// Archive 追加一个归档版本；重复归档不制造无意义的新版本。
func (s *Store) Archive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.readCurrent(id)
	if err != nil {
		return err
	}
	if current.Archived {
		return nil
	}
	current.Archived = true
	current.Revision++
	return s.writeRevision(current)
}

func (s *Store) readCurrent(id string) (presets.Revision, error) {
	numbers, err := s.revisionNumbers(id)
	if err != nil {
		return presets.Revision{}, err
	}
	if len(numbers) == 0 {
		return presets.Revision{}, fmt.Errorf("Agent 模式 %s 没有版本", id)
	}
	return s.readRevision(id, numbers[len(numbers)-1])
}

func (s *Store) readRevision(id string, number int) (presets.Revision, error) {
	data, err := os.ReadFile(s.revisionPath(id, number))
	if err != nil {
		return presets.Revision{}, fmt.Errorf("读取 Agent 模式 %s 的版本 %d 失败：%w", id, number, err)
	}
	var revision presets.Revision
	err = json.Unmarshal(data, &revision)
	if err != nil {
		return presets.Revision{}, fmt.Errorf("解开 Agent 模式 %s 的版本 %d 失败：%w", id, number, err)
	}
	if revision.ID != id || revision.Revision != number {
		return presets.Revision{}, fmt.Errorf("Agent 模式文件 %s 不属于 %s 的版本 %d", s.revisionPath(id, number), id, number)
	}
	err = presets.Validate(revision)
	if err != nil {
		return presets.Revision{}, err
	}
	return revision, nil
}

func (s *Store) revisionNumbers(id string) ([]int, error) {
	entries, err := os.ReadDir(s.directory(id))
	if err != nil {
		return nil, fmt.Errorf("列出 Agent 模式 %s 的版本失败：%w", id, err)
	}
	numbers := make([]int, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("Agent 模式 %s 有不合法的版本文件 %s", id, entry.Name())
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		number, err := strconv.Atoi(name)
		if err != nil || number < 1 || strconv.Itoa(number) != name {
			return nil, fmt.Errorf("Agent 模式 %s 有不合法的版本文件 %s", id, entry.Name())
		}
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	return numbers, nil
}

func (s *Store) writeRevision(revision presets.Revision) error {
	data, err := json.Marshal(revision)
	if err != nil {
		return fmt.Errorf("编码 Agent 模式 %s 的版本 %d 失败：%w", revision.ID, revision.Revision, err)
	}
	temp, err := os.CreateTemp(s.root, ".preset-revision-*")
	if err != nil {
		return fmt.Errorf("创建 Agent 模式 %s 的版本 %d 临时文件失败：%w", revision.ID, revision.Revision, err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	err = temp.Chmod(0o600)
	if err == nil {
		err = writeFull(temp, data)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("写 Agent 模式 %s 的版本 %d 失败：%w", revision.ID, revision.Revision, err)
	}
	err = os.Rename(tempName, s.revisionPath(revision.ID, revision.Revision))
	if err != nil {
		return fmt.Errorf("追加 Agent 模式 %s 的版本 %d 失败：%w", revision.ID, revision.Revision, err)
	}
	return nil
}

func (s *Store) directory(id string) string {
	return filepath.Join(s.root, base64.RawURLEncoding.EncodeToString([]byte(id)))
}

func (s *Store) revisionPath(id string, number int) string {
	return filepath.Join(s.directory(id), strconv.Itoa(number)+".json")
}

func decodeID(name string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(name)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errors.New("id 不能为空")
	}
	return string(data), nil
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
