package hooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"harness/internal/machine"
	"harness/internal/persist"
	"harness/internal/tools"
)

const (
	maxConfigBytes = 64 << 10
	maxInputBytes  = 256 << 10
	maxOutputBytes = 64 << 10
	defaultTimeout = 10
)

// ErrSettings 表示提交的 Hook 设置不合法。
var ErrSettings = errors.New("hooks: invalid settings")

// 活对象。读取全局和项目前置 Hook，并记录最近一次运行故障。
type Service struct {
	files   *persist.Files
	machine machine.FileSystem
	mu      sync.Mutex
	last    string
}

// NewService 绑定文件来源；脚本只在 Check 时启动。
func NewService(files *persist.Files, filesystem machine.FileSystem) (*Service, error) {
	if files == nil || filesystem == nil {
		return nil, fmt.Errorf("hooks: missing file service")
	}
	scope, err := files.Scope("hooks")
	if err != nil {
		return nil, err
	}
	return &Service{files: scope, machine: filesystem}, nil
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func decode(body []byte) ([]Hook, error) {
	var config struct {
		Hooks []Hook `json:"hooks"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("invalid Hook configuration: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("invalid Hook configuration: trailing content")
	}
	if err := validate(config.Hooks); err != nil {
		return nil, err
	}
	if config.Hooks == nil {
		config.Hooks = []Hook{}
	}
	for index := range config.Hooks {
		if config.Hooks[index].Tools == nil {
			config.Hooks[index].Tools = []string{}
		}
		if config.Hooks[index].Args == nil {
			config.Hooks[index].Args = []string{}
		}
	}
	return config.Hooks, nil
}

func validate(hooks []Hook) error {
	seen := make(map[string]bool, len(hooks))
	for _, hook := range hooks {
		if strings.TrimSpace(hook.Name) == "" || strings.TrimSpace(hook.Command) == "" {
			return fmt.Errorf("hooks: name and command are required")
		}
		if seen[hook.Name] {
			return fmt.Errorf("hooks: duplicate name %q", hook.Name)
		}
		seen[hook.Name] = true
		if hook.TimeoutSeconds < 0 || hook.TimeoutSeconds > 60 {
			return fmt.Errorf("hooks: %q timeout must be 1–60 seconds or zero for default", hook.Name)
		}
		for _, name := range hook.Tools {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("hooks: %q has an empty tool name", hook.Name)
			}
		}
	}
	return nil
}

func projectPath(workspace string) (string, string, error) {
	if workspace == "" {
		return "", "", nil
	}
	real, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", "", fmt.Errorf("hooks: resolve workspace: %w", err)
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("hooks: workspace is not a directory")
	}
	return real, filepath.Join(real, ".harness", "hooks.json"), nil
}

func (s *Service) readGlobal() Source {
	result := Source{Hooks: []Hook{}}
	body, err := s.files.Read("settings.json")
	if errors.Is(err, os.ErrNotExist) {
		return result
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Hash = digest(body)
	if len(body) > maxConfigBytes {
		result.Error = "Hook configuration is too large"
		return result
	}
	hooks, err := decode(body)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Hooks = hooks
	return result
}

func (s *Service) readProject(workspace string) (Source, string) {
	result := Source{Hooks: []Hook{}}
	real, path, err := projectPath(workspace)
	if err != nil {
		result.Error = err.Error()
		return result, ""
	}
	if path == "" {
		return result, ""
	}
	content, err := s.machine.ReadFileVersion(path, maxConfigBytes)
	if errors.Is(err, os.ErrNotExist) {
		return result, real
	}
	if err != nil {
		result.Error = err.Error()
		return result, real
	}
	result.Hash = content.Hash
	hooks, err := decode(content.Data)
	if err != nil {
		result.Error = err.Error()
		return result, real
	}
	result.Hooks = hooks
	return result, real
}

func (s *Service) readTrust() (map[string]string, error) {
	body, err := s.files.Read("trust.json")
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	var trusted map[string]string
	if err := json.Unmarshal(body, &trusted); err != nil {
		return nil, fmt.Errorf("hooks: invalid project trust: %w", err)
	}
	if trusted == nil {
		trusted = make(map[string]string)
	}
	return trusted, nil
}

// View 读取当前文件版本和信任状态；文件内容错误会显示在对应来源。
func (s *Service) View(workspace string) (View, error) {
	global := s.readGlobal()
	project, real := s.readProject(workspace)
	s.mu.Lock()
	defer s.mu.Unlock()
	trusted, err := s.readTrust()
	if err != nil {
		return View{}, err
	}
	return View{
		Global: global, Project: project, Workspace: real,
		Trusted:   project.Hash == "" || trusted[real] == project.Hash,
		LastError: s.last,
	}, nil
}

// Save 用读取时的哈希写入配置；项目页面保存同时信任新版本。
func (s *Service) Save(input SaveInput) (View, error) {
	if err := validate(input.Hooks); err != nil {
		return View{}, fmt.Errorf("%w: %v", ErrSettings, err)
	}
	if input.Hooks == nil {
		input.Hooks = []Hook{}
	}
	for index := range input.Hooks {
		if input.Hooks[index].Tools == nil {
			input.Hooks[index].Tools = []string{}
		}
		if input.Hooks[index].Args == nil {
			input.Hooks[index].Args = []string{}
		}
	}
	body, err := json.MarshalIndent(struct {
		Hooks []Hook `json:"hooks"`
	}{Hooks: input.Hooks}, "", "  ")
	if err != nil {
		return View{}, err
	}
	if len(body) > maxConfigBytes {
		return View{}, fmt.Errorf("%w: configuration is too large", ErrSettings)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch input.Scope {
	case "global":
		current := s.readGlobal()
		if current.Hash != input.Hash || (current.Error != "" && current.Hash == "") {
			return View{}, machine.ErrFileConflict
		}
		err = s.files.Write("settings.json", body)
	case "project":
		real, path, pathErr := projectPath(input.Workspace)
		if pathErr != nil {
			return View{}, fmt.Errorf("%w: %v", ErrSettings, pathErr)
		}
		if path == "" {
			return View{}, fmt.Errorf("%w: workspace is required", ErrSettings)
		}
		_, err = s.machine.WriteFileIfUnchanged(path, body, input.Hash)
		if err == nil {
			trusted, readErr := s.readTrust()
			if readErr != nil {
				return View{}, readErr
			}
			trusted[real] = digest(body)
			trustBody, marshalErr := json.Marshal(trusted)
			if marshalErr != nil {
				return View{}, marshalErr
			}
			err = s.files.Write("trust.json", trustBody)
		}
	default:
		return View{}, fmt.Errorf("%w: unknown scope %q", ErrSettings, input.Scope)
	}
	if err != nil {
		return View{}, err
	}
	// 这里仍持锁，避免并发保存把响应的版本提前换掉。
	global := s.readGlobal()
	project, real := s.readProject(input.Workspace)
	trusted, err := s.readTrust()
	if err != nil {
		return View{}, err
	}
	return View{Global: global, Project: project, Workspace: real, Trusted: project.Hash == "" || trusted[real] == project.Hash, LastError: s.last}, nil
}

// Trust 确认页面刚读到的项目配置版本。
func (s *Service) Trust(input TrustInput) (View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, real := s.readProject(input.Workspace)
	if project.Error != "" || project.Hash == "" || project.Hash != input.Hash || real == "" {
		return View{}, machine.ErrFileConflict
	}
	trusted, err := s.readTrust()
	if err != nil {
		return View{}, err
	}
	trusted[real] = project.Hash
	body, err := json.Marshal(trusted)
	if err != nil {
		return View{}, err
	}
	if err := s.files.Write("trust.json", body); err != nil {
		return View{}, err
	}
	return View{Global: s.readGlobal(), Project: project, Workspace: real, Trusted: true, LastError: s.last}, nil
}

func (s *Service) failure(message string, report func(string)) {
	characters := []rune(message)
	if len(characters) > 500 {
		message = string(characters[:500]) + "…"
	}
	s.mu.Lock()
	s.last = message
	s.mu.Unlock()
	if report != nil {
		report(message)
	}
}

// Check 顺序执行匹配 Hook；只有明确 deny 才阻止工具。
func (s *Service) Check(ctx context.Context, call tools.Call, report func(string)) (string, error) {
	global := s.readGlobal()
	project, real := s.readProject(call.Workspace)
	if global.Error != "" {
		s.failure("全局 Hook 配置错误："+global.Error, report)
	}
	if project.Error != "" {
		s.failure("项目 Hook 配置错误："+project.Error, report)
	}
	var projectHooks []Hook
	if project.Error == "" && project.Hash != "" {
		s.mu.Lock()
		trusted, err := s.readTrust()
		s.mu.Unlock()
		if err != nil {
			s.failure("项目 Hook 信任记录读取失败："+err.Error(), report)
		} else if trusted[real] != project.Hash {
			s.failure("项目 Hook 配置已变更，请在设置中确认", report)
		} else {
			projectHooks = project.Hooks
		}
	}
	for _, hook := range append(global.Hooks, projectHooks...) {
		if !hook.Enabled || !matches(hook.Tools, call.Name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		reason, err := run(ctx, hook, call)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if err != nil {
			s.failure(fmt.Sprintf("Hook %q 失败：%v", hook.Name, err), report)
			continue
		}
		if reason != "" {
			return reason, nil
		}
	}
	return "", ctx.Err()
}

func matches(names []string, target string) bool {
	if len(names) == 0 {
		return true
	}
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

type limitedBuffer struct {
	bytes.Buffer
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > maxOutputBytes {
		return 0, fmt.Errorf("output exceeds %d bytes", maxOutputBytes)
	}
	return b.Buffer.Write(data)
}

func run(ctx context.Context, hook Hook, call tools.Call) (string, error) {
	input, err := json.Marshal(struct {
		Name      string          `json:"tool_name"`
		Input     json.RawMessage `json:"tool_input"`
		CallID    string          `json:"tool_call_id"`
		Workspace string          `json:"workspace"`
	}{call.Name, call.Arguments, call.ToolCallID, call.Workspace})
	if err != nil {
		return "", err
	}
	if len(input) > maxInputBytes {
		return "", fmt.Errorf("input exceeds %d bytes", maxInputBytes)
	}
	timeout := hook.TimeoutSeconds
	if timeout == 0 {
		timeout = defaultTimeout
	}
	hookCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(hookCtx, hook.Command, hook.Args...)
	prepareCommand(cmd)
	cmd.Dir = call.Workspace
	cmd.Stdin = bytes.NewReader(input)
	cmd.WaitDelay = time.Second
	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if errors.Is(hookCtx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("timed out after %d seconds", timeout)
	}
	if err != nil {
		return "", fmt.Errorf("command: %w; stderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.Len() == 0 {
		return "", nil
	}
	var decision struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decision); err != nil {
		return "", fmt.Errorf("invalid stdout JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("stdout contains trailing content")
	}
	if decision.Decision != "deny" || strings.TrimSpace(decision.Reason) == "" {
		return "", fmt.Errorf("stdout must be empty or a deny decision with reason")
	}
	return decision.Reason, nil
}
