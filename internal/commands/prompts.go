package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"harness/internal/machine"
	"harness/internal/persist"
)

const maxPromptFileBytes = 256 << 10

var promptNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)

// 活对象。用户与工作区的提示词命令；锁保护同进程读改写。
type PromptStore struct {
	mu      sync.Mutex
	user    *persist.Files
	machine machine.FileSystem
}

// NewPromptStore 接入受保护的用户目录和工作区文件系统。
func NewPromptStore(files *persist.Files, filesystem machine.FileSystem) (*PromptStore, error) {
	if files == nil || filesystem == nil {
		return nil, fmt.Errorf("commands: missing prompt storage")
	}
	user, err := files.Scope("commands")
	if err != nil {
		return nil, err
	}
	return &PromptStore{user: user, machine: filesystem}, nil
}

func promptHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func promptPath(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", fmt.Errorf("commands: workspace is required")
	}
	real, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("commands: resolve workspace: %w", err)
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("commands: workspace is not a directory")
	}
	return filepath.Join(real, ".harness", "commands.json"), nil
}

func validatePrompts(items []Prompt) error {
	if len(items) > 100 {
		return fmt.Errorf("commands: too many prompt commands")
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if !promptNamePattern.MatchString(item.Name) || strings.TrimSpace(item.Prompt) == "" || len(item.Prompt) > 32<<10 || len(item.Description) > 200 || len(item.ArgumentHint) > 100 {
			return fmt.Errorf("commands: invalid prompt command %q", item.Name)
		}
		name := strings.ToLower(item.Name)
		if seen[name] {
			return fmt.Errorf("commands: duplicate prompt command %q", item.Name)
		}
		seen[name] = true
	}
	return nil
}

func (s *PromptStore) read(scope, workspace string) (PromptFile, error) {
	var body []byte
	var err error
	switch scope {
	case "user":
		body, err = s.user.Read("settings.json")
	case "workspace":
		var path string
		path, err = promptPath(workspace)
		if err != nil {
			return PromptFile{}, err
		}
		var content machine.FileContent
		content, err = s.machine.ReadFileVersion(path, maxPromptFileBytes)
		body = content.Data
	default:
		return PromptFile{}, fmt.Errorf("commands: unknown scope %q", scope)
	}
	if errors.Is(err, os.ErrNotExist) {
		return PromptFile{Commands: []Prompt{}}, nil
	}
	if err != nil {
		return PromptFile{}, err
	}
	if len(body) > maxPromptFileBytes {
		return PromptFile{}, fmt.Errorf("commands: configuration is too large")
	}
	var data struct {
		Commands []Prompt `json:"commands"`
	}
	if err = json.Unmarshal(body, &data); err != nil {
		return PromptFile{}, fmt.Errorf("commands: invalid %s configuration: %w", scope, err)
	}
	if err = validatePrompts(data.Commands); err != nil {
		return PromptFile{}, err
	}
	if data.Commands == nil {
		data.Commands = []Prompt{}
	}
	for index := range data.Commands {
		data.Commands[index].ID = scope + ":" + data.Commands[index].Name
		data.Commands[index].Scope = scope
		data.Commands[index].Source = "local"
	}
	return PromptFile{Commands: data.Commands, Hash: promptHash(body)}, nil
}

// Read 返回指定作用域的配置和内容版本。
func (s *PromptStore) Read(scope, workspace string) (PromptFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(scope, workspace)
}

// List 将两个作用域并列返回，不按名称覆盖。
func (s *PromptStore) List(workspace string) ([]Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, err := s.read("user", "")
	if err != nil {
		return nil, err
	}
	if workspace == "" {
		return user.Commands, nil
	}
	project, err := s.read("workspace", workspace)
	if err != nil {
		return nil, err
	}
	return append(project.Commands, user.Commands...), nil
}

// Resolve 按稳定来源 ID 读取最新命令，配置变化后不会误用同名的另一来源。
func (s *PromptStore) Resolve(workspace, id string) (Prompt, error) {
	scope, name, ok := strings.Cut(id, ":")
	if !ok || (scope != "user" && scope != "workspace") {
		return Prompt{}, fmt.Errorf("commands: unknown prompt command %q", id)
	}
	file, err := s.Read(scope, workspace)
	if err != nil {
		return Prompt{}, err
	}
	for _, item := range file.Commands {
		if item.Name == name {
			return item, nil
		}
	}
	return Prompt{}, fmt.Errorf("commands: prompt command %q no longer exists", id)
}

// Save 在同一作用域创建或更新一条命令，拒绝覆盖外部修改。
func (s *PromptStore) Save(scope, workspace, expectedHash string, item Prompt, create bool) (PromptFile, error) {
	if err := validatePrompts([]Prompt{item}); err != nil {
		return PromptFile{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.read(scope, workspace)
	if err != nil {
		return PromptFile{}, err
	}
	if file.Hash != expectedHash {
		return PromptFile{}, machine.ErrFileConflict
	}
	item.ID, item.Scope, item.Source = "", "", ""
	updated := false
	for index := range file.Commands {
		if strings.EqualFold(file.Commands[index].Name, item.Name) {
			if create {
				return PromptFile{}, fmt.Errorf("commands: /%s already exists in %s", item.Name, scope)
			}
			file.Commands[index] = item
			updated = true
			break
		}
	}
	if !updated {
		if !create {
			return PromptFile{}, fmt.Errorf("commands: /%s no longer exists in %s", item.Name, scope)
		}
		file.Commands = append(file.Commands, item)
	}
	return s.write(scope, workspace, expectedHash, file.Commands)
}

// Delete 仅删除指定作用域的一条命令。
func (s *PromptStore) Delete(scope, workspace, expectedHash, name string) (PromptFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.read(scope, workspace)
	if err != nil {
		return PromptFile{}, err
	}
	if file.Hash != expectedHash {
		return PromptFile{}, machine.ErrFileConflict
	}
	index := -1
	for i, item := range file.Commands {
		if item.Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		return PromptFile{}, fmt.Errorf("commands: prompt command %q no longer exists", name)
	}
	file.Commands = append(file.Commands[:index], file.Commands[index+1:]...)
	return s.write(scope, workspace, expectedHash, file.Commands)
}

func (s *PromptStore) write(scope, workspace, expectedHash string, items []Prompt) (PromptFile, error) {
	if err := validatePrompts(items); err != nil {
		return PromptFile{}, err
	}
	clean := make([]Prompt, len(items))
	for index, item := range items {
		clean[index] = Prompt{Name: item.Name, Description: item.Description, ArgumentHint: item.ArgumentHint, Prompt: item.Prompt}
	}
	body, err := json.MarshalIndent(struct {
		Commands []Prompt `json:"commands"`
	}{Commands: clean}, "", "  ")
	if err != nil {
		return PromptFile{}, err
	}
	if len(body) > maxPromptFileBytes {
		return PromptFile{}, fmt.Errorf("commands: configuration is too large")
	}
	if scope == "user" {
		err = s.user.Write("settings.json", body)
	} else {
		var path string
		path, err = promptPath(workspace)
		if err != nil {
			return PromptFile{}, err
		}
		_, err = s.machine.WriteFileIfUnchanged(path, body, expectedHash)
	}
	if err != nil {
		return PromptFile{}, err
	}
	return s.read(scope, workspace)
}

// Expand 用最新配置展开用户显式选中的命令；参数提示只影响界面文案。
func Expand(item Prompt, text string) (string, error) {
	invocation := "/" + item.Name
	if !strings.HasPrefix(text, invocation) || (len(text) > len(invocation) && text[len(invocation)] != ' ' && text[len(invocation)] != '\n') {
		return "", fmt.Errorf("commands: input no longer matches /%s", item.Name)
	}
	args := strings.TrimSpace(text[len(invocation):])
	if strings.Contains(item.Prompt, "$ARGUMENTS") {
		return strings.ReplaceAll(item.Prompt, "$ARGUMENTS", args), nil
	}
	if args != "" {
		return item.Prompt + "\n\n" + args, nil
	}
	return item.Prompt, nil
}
