package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalid = errors.New("mcp: invalid settings")
	ErrChanged = errors.New("mcp: settings changed")
	ErrMissing = errors.New("mcp: server missing")
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var headerNamePattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// 数据。设置页可见的 Server 摘要；不包含会展开或直接承载密钥的值。
type ServerView struct {
	Name         string   `json:"name"`
	Scope        string   `json:"scope"`
	Source       string   `json:"source"`
	Type         string   `json:"type"`
	Enabled      bool     `json:"enabled"`
	Overridden   bool     `json:"overridden"`
	HasCommand   bool     `json:"hasCommand"`
	HasURL       bool     `json:"hasURL"`
	HasCWD       bool     `json:"hasCWD"`
	ArgCount     int      `json:"argCount"`
	EnvKeys      []string `json:"envKeys"`
	HeaderKeys   []string `json:"headerKeys"`
	IncludeTools []string `json:"includeTools"`
	ExcludeTools []string `json:"excludeTools"`
	Status       string   `json:"status"`
	Error        string   `json:"error,omitempty"`
	Tools        []string `json:"tools"`
}

// 数据。设置页一次读取的安全视图。
type SettingsView struct {
	Revision     string       `json:"revision"`
	Global       []ServerView `json:"global"`
	Project      []ServerView `json:"project"`
	GlobalError  string       `json:"globalError,omitempty"`
	ProjectError string       `json:"projectError,omitempty"`
	GlobalPath   string       `json:"globalPath"`
	ProjectPaths []string     `json:"projectPaths"`
}

// 数据。保存一条全局 Server；指针字段省略时保留旧值，空值为显式清除。
type SaveInput struct {
	Name         string             `json:"name"`
	Revision     string             `json:"revision"`
	Create       bool               `json:"create"`
	Type         *string            `json:"type,omitempty"`
	Enabled      *bool              `json:"enabled,omitempty"`
	Command      *string            `json:"command,omitempty"`
	Args         *[]string          `json:"args,omitempty"`
	Env          map[string]*string `json:"env,omitempty"`
	CWD          *string            `json:"cwd,omitempty"`
	URL          *string            `json:"url,omitempty"`
	Headers      map[string]*string `json:"headers,omitempty"`
	IncludeTools *[]string          `json:"includeTools,omitempty"`
	ExcludeTools *[]string          `json:"excludeTools,omitempty"`
}

type plannedServer struct {
	name    string
	config  serverConfig
	project bool
	source  string
	spec    *serverSpec
	err     string
}

type sourcePlan struct {
	version        string
	globalRevision string
	projectVersion string
	projectSource  string
	projectDigest  string
	projectError   string
	entries        []plannedServer
	global         []plannedServer
}

func revision(data []byte, exists bool) string {
	if !exists {
		return "missing"
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (p *Provider) globalFile() ([]byte, bool, error) {
	if p.userOverride != nil {
		data, err := json.Marshal(p.userOverride)
		return data, true, err
	}
	data, err := p.files.Read("mcp.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func (p *Provider) globalConfig() (configFile, string, string, error) {
	data, exists, err := p.globalFile()
	if err != nil {
		return configFile{}, "", "", err
	}
	version := revision(data, exists)
	if !exists {
		return configFile{MCPServers: map[string]serverConfig{}}, version, "", nil
	}
	config, err := parseConfig(data, "mcp.json")
	if err != nil {
		return configFile{MCPServers: map[string]serverConfig{}}, version, "全局 MCP 配置格式错误", nil
	}
	return config, version, "", nil
}

func readProjectConfig(workspace string) (configFile, []string, string, string) {
	merged := configFile{MCPServers: map[string]serverConfig{}}
	if workspace == "" {
		return merged, nil, "", ""
	}
	realPath, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return merged, nil, "", "项目路径不可用"
	}
	paths := []string{filepath.Join(realPath, ".mcp.json"), filepath.Join(realPath, ".harness", "mcp.json")}
	var versionParts []string
	var found []string
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if errors.Is(readErr, os.ErrNotExist) {
			versionParts = append(versionParts, "missing")
			continue
		}
		if readErr != nil {
			return merged, paths, "", "项目 MCP 配置无法读取"
		}
		versionParts = append(versionParts, revision(data, true))
		found = append(found, path)
		config, parseErr := parseConfig(data, path)
		if parseErr != nil {
			return merged, paths, realPath + ":" + strings.Join(versionParts, ":"), "项目 MCP 配置格式错误"
		}
		for name, server := range config.MCPServers {
			merged.MCPServers[name] = server
		}
	}
	return merged, found, realPath + ":" + strings.Join(versionParts, ":"), ""
}

func (p *Provider) plan(workspace string) (sourcePlan, error) {
	global, globalVersion, globalError, err := p.globalConfig()
	if err != nil {
		return sourcePlan{}, err
	}
	project, paths, projectVersion, projectError := readProjectConfig(workspace)
	plan := sourcePlan{globalRevision: globalVersion, projectVersion: projectVersion,
		projectSource: strings.Join(paths, "、"), projectError: projectError}
	if globalError != "" {
		global = configFile{MCPServers: map[string]serverConfig{}}
	}
	for name, config := range global.MCPServers {
		plan.global = append(plan.global, plannedServer{name: name, config: config})
	}
	if projectError == "" {
		for name, config := range global.MCPServers {
			if _, overridden := project.MCPServers[name]; !overridden {
				plan.entries = append(plan.entries, plannedServer{name: name, config: config})
			}
		}
		for name, config := range project.MCPServers {
			plan.entries = append(plan.entries, plannedServer{name: name, config: config, project: true, source: plan.projectSource})
		}
	}
	sort.Slice(plan.entries, func(i, j int) bool { return plan.entries[i].name < plan.entries[j].name })
	sort.Slice(plan.global, func(i, j int) bool { return plan.global[i].name < plan.global[j].name })
	for i := range plan.entries {
		entry := &plan.entries[i]
		if !entry.config.isEnabled() {
			continue
		}
		spec, normalizeErr := normalizeServer(entry.name, entry.config, p.launchDir)
		if entry.project {
			realPath, pathErr := filepath.EvalSymlinks(workspace)
			if pathErr != nil {
				entry.err = "项目路径不可用"
				continue
			}
			spec, normalizeErr = normalizeServer(entry.name, entry.config, realPath)
		}
		if normalizeErr != nil {
			entry.err = safeConfigError(normalizeErr)
			continue
		}
		entry.spec = &spec
	}
	for i := range plan.global {
		entry := &plan.global[i]
		if !entry.config.isEnabled() {
			continue
		}
		spec, normalizeErr := normalizeServer(entry.name, entry.config, p.launchDir)
		if normalizeErr != nil {
			entry.err = safeConfigError(normalizeErr)
			continue
		}
		entry.spec = &spec
	}
	projectSpecs := make([]serverSpec, 0)
	for _, entry := range plan.entries {
		if entry.project && entry.spec != nil {
			projectSpecs = append(projectSpecs, *entry.spec)
		}
	}
	projectBody, err := json.Marshal(struct {
		Source  string
		Version string
		Config  configFile
		Specs   []serverSpec
	}{plan.projectSource, projectVersion, project, projectSpecs})
	if err != nil {
		return sourcePlan{}, err
	}
	projectHash := sha256.Sum256(projectBody)
	plan.projectDigest = hex.EncodeToString(projectHash[:])
	resolved := make([]any, 0, len(plan.entries))
	for _, entry := range plan.entries {
		resolved = append(resolved, struct {
			Name  string
			Spec  *serverSpec
			Error string
		}{entry.name, entry.spec, entry.err})
	}
	versionBody, err := json.Marshal(struct {
		Global  string
		Project string
		Source  string
		Entries []any
	}{globalVersion, projectVersion, plan.projectSource, resolved})
	if err != nil {
		return sourcePlan{}, err
	}
	versionHash := sha256.Sum256(versionBody)
	plan.version = hex.EncodeToString(versionHash[:])
	return plan, nil
}

func safeConfigError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if strings.Contains(text, "environment variable") {
		return "引用的环境变量不可用"
	}
	return "Server 配置无效"
}

func keys(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func serverView(name, scope, source string, config serverConfig) ServerView {
	view := ServerView{Name: name, Scope: scope, Source: source, Type: config.Type,
		Enabled: config.isEnabled(), HasCommand: config.Command != "", HasURL: config.URL != "",
		HasCWD: config.CWD != "", ArgCount: len(config.Args), EnvKeys: keys(config.Env),
		HeaderKeys: keys(config.Headers), IncludeTools: []string{}, ExcludeTools: append([]string{}, config.ExcludeTools...),
		Tools: []string{}, Status: "saved"}
	if config.IncludeTools != nil {
		view.IncludeTools = append([]string{}, (*config.IncludeTools)...)
	}
	if !view.Enabled {
		view.Status = "disabled"
	}
	return view
}

// ReadSettings 只解析配置和已有状态，不连接 Server，也不触发项目审批。
func (p *Provider) ReadSettings(workspace string) (SettingsView, error) {
	global, version, globalError, err := p.globalConfig()
	if err != nil {
		return SettingsView{}, err
	}
	path := ""
	if p.files != nil {
		path, err = p.files.Path("mcp.json")
		if err != nil {
			return SettingsView{}, err
		}
	}
	project, projectPaths, projectVersion, projectError := readProjectConfig(workspace)
	if projectPaths == nil {
		projectPaths = []string{}
	}
	freshPlan, planErr := p.plan(workspace)
	view := SettingsView{Revision: version, Global: []ServerView{}, Project: []ServerView{},
		GlobalError: globalError, ProjectError: projectError, GlobalPath: path, ProjectPaths: projectPaths}
	p.mu.Lock()
	state := p.current[workspace]
	globalState := p.current[""]
	if state != nil && (planErr != nil || !state.built || state.plan.version != freshPlan.version || state.plan.globalRevision != version ||
		state.plan.projectVersion != projectVersion || state.plan.projectError != projectError ||
		state.plan.projectSource != strings.Join(projectPaths, "、")) {
		state = nil
	}
	if globalState != nil && (!globalState.built || globalState.plan.globalRevision != version) {
		globalState = nil
	}
	p.mu.Unlock()
	for name, config := range global.MCPServers {
		item := serverView(name, "global", path, config)
		_, item.Overridden = project.MCPServers[name]
		if state != nil {
			p.applyStatus(&item, state)
		} else {
			p.applyStatus(&item, globalState)
		}
		if item.Status == "saved" && planErr == nil {
			for _, planned := range freshPlan.global {
				if planned.name == name && planned.err != "" {
					item.Status, item.Error = "invalid", planned.err
				}
			}
		}
		view.Global = append(view.Global, item)
	}
	for name, config := range project.MCPServers {
		source := ""
		for _, candidate := range projectPaths {
			if parsed, _, parseErr := readConfig(candidate); parseErr == nil {
				if _, ok := parsed.MCPServers[name]; ok {
					source = candidate
				}
			}
		}
		item := serverView(name, "project", source, config)
		p.applyStatus(&item, state)
		if item.Status == "saved" && planErr == nil {
			for _, planned := range freshPlan.entries {
				if planned.project && planned.name == name && planned.err != "" {
					item.Status, item.Error = "invalid", planned.err
				}
			}
		}
		view.Project = append(view.Project, item)
	}
	sort.Slice(view.Global, func(i, j int) bool { return view.Global[i].Name < view.Global[j].Name })
	sort.Slice(view.Project, func(i, j int) bool { return view.Project[i].Name < view.Project[j].Name })
	return view, nil
}

func (p *Provider) applyStatus(view *ServerView, state *workspaceState) {
	if state == nil || !state.built || view.Status == "disabled" || view.Overridden {
		return
	}
	status, ok := state.status[view.Name]
	if !ok {
		return
	}
	view.Status, view.Error = status.state, status.message
	view.Tools = append([]string{}, status.tools...)
}

func applyChanges(config *serverConfig, input SaveInput) {
	if input.Type != nil && *input.Type != config.Type {
		*config = serverConfig{Type: *input.Type}
	}
	if input.Enabled != nil {
		config.Enabled = input.Enabled
	}
	if input.Command != nil {
		config.Command = *input.Command
	}
	if input.Args != nil {
		config.Args = append([]string(nil), (*input.Args)...)
	}
	if input.CWD != nil {
		config.CWD = *input.CWD
	}
	if input.URL != nil {
		config.URL = *input.URL
	}
	if input.IncludeTools != nil {
		if len(*input.IncludeTools) == 0 {
			config.IncludeTools = nil
		} else {
			config.IncludeTools = copyStringList(input.IncludeTools)
		}
	}
	if input.ExcludeTools != nil {
		config.ExcludeTools = append([]string(nil), (*input.ExcludeTools)...)
	}
	if config.Env == nil {
		config.Env = map[string]string{}
	}
	for key, value := range input.Env {
		if value == nil {
			delete(config.Env, key)
		} else {
			config.Env[key] = *value
		}
	}
	if config.Headers == nil {
		config.Headers = map[string]string{}
	}
	for key, value := range input.Headers {
		if value == nil {
			delete(config.Headers, key)
		} else {
			config.Headers[key] = *value
		}
	}
}

func validateRaw(name string, config serverConfig) error {
	if !serverNamePattern.MatchString(name) {
		return fmt.Errorf("%w: Server 名称只允许字母、数字、下划线和连字符", ErrInvalid)
	}
	if !config.isEnabled() {
		return nil
	}
	if config.Type == "streamable-http" {
		config.Type = transportHTTP
	}
	switch config.Type {
	case transportStdio:
		if strings.TrimSpace(config.Command) == "" || config.URL != "" || len(config.Headers) != 0 {
			return fmt.Errorf("%w: STDIO 需要命令，不能填写 URL 或 Header", ErrInvalid)
		}
	case transportHTTP:
		if strings.TrimSpace(config.URL) == "" || config.Command != "" || len(config.Args) != 0 || len(config.Env) != 0 || config.CWD != "" {
			return fmt.Errorf("%w: HTTP 需要 URL，不能填写命令配置", ErrInvalid)
		}
		candidate := environmentPattern.ReplaceAllString(config.URL, "value")
		parsed, err := url.Parse(candidate)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return fmt.Errorf("%w: HTTP URL 无效", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: 不支持的传输方式", ErrInvalid)
	}
	for key := range config.Env {
		if !environmentNamePattern.MatchString(key) {
			return fmt.Errorf("%w: 环境变量名称无效", ErrInvalid)
		}
	}
	for key := range config.Headers {
		if !headerNamePattern.MatchString(key) {
			return fmt.Errorf("%w: Header 名称无效", ErrInvalid)
		}
	}
	return nil
}

func (p *Provider) writeGlobal(expected string, change func(*configFile) error) error {
	p.configMu.Lock()
	defer p.configMu.Unlock()
	data, exists, err := p.globalFile()
	if err != nil {
		return err
	}
	if revision(data, exists) != expected {
		return ErrChanged
	}
	config := configFile{MCPServers: map[string]serverConfig{}}
	if exists {
		config, err = parseConfig(data, "mcp.json")
		if err != nil {
			return fmt.Errorf("%w: 全局配置损坏，请先修复或重建", ErrInvalid)
		}
	}
	if err = change(&config); err != nil {
		return err
	}
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return p.files.Write("mcp.json", data)
}

// Save 保存一条全局 Server；连接结果另行反映在读取视图中。
func (p *Provider) Save(ctx context.Context, input SaveInput) (SettingsView, error) {
	err := p.writeGlobal(input.Revision, func(config *configFile) error {
		old, exists := config.MCPServers[input.Name]
		if input.Create == exists {
			return ErrChanged
		}
		applyChanges(&old, input)
		if err := validateRaw(input.Name, old); err != nil {
			return err
		}
		config.MCPServers[input.Name] = old
		return nil
	})
	if err != nil {
		return SettingsView{}, err
	}
	p.refreshGlobal(ctx)
	return p.ReadSettings("")
}

// Delete 删除一条全局 Server，正在使用旧版本的 Run 保留旧连接。
func (p *Provider) Delete(ctx context.Context, name, expected string) (SettingsView, error) {
	err := p.writeGlobal(expected, func(config *configFile) error {
		if _, ok := config.MCPServers[name]; !ok {
			return ErrMissing
		}
		delete(config.MCPServers, name)
		return nil
	})
	if err != nil {
		return SettingsView{}, err
	}
	p.refreshGlobal(ctx)
	return p.ReadSettings("")
}

// Retry 重新连接全局配置，不改变磁盘版本。
func (p *Provider) Retry(ctx context.Context, name string) (SettingsView, error) {
	config, _, bad, err := p.globalConfig()
	if err != nil {
		return SettingsView{}, err
	}
	if bad != "" {
		return SettingsView{}, fmt.Errorf("%w: 全局配置损坏", ErrInvalid)
	}
	if _, ok := config.MCPServers[name]; !ok {
		return SettingsView{}, ErrMissing
	}
	p.refreshGlobal(ctx)
	return p.ReadSettings("")
}

// ResetInvalid 仅在用户明确要求且版本匹配时备份损坏文件并重建。
func (p *Provider) ResetInvalid(ctx context.Context, expected string) (SettingsView, error) {
	p.configMu.Lock()
	data, exists, err := p.globalFile()
	if err == nil && revision(data, exists) != expected {
		err = ErrChanged
	}
	if err == nil && !exists {
		err = fmt.Errorf("%w: 配置文件不存在", ErrInvalid)
	}
	if err == nil {
		if _, parseErr := parseConfig(data, "mcp.json"); parseErr == nil {
			err = fmt.Errorf("%w: 配置文件没有损坏", ErrInvalid)
		}
	}
	if err == nil {
		backup := fmt.Sprintf("mcp.json.backup-%d", time.Now().UnixNano())
		err = p.files.Write(backup, data)
	}
	if err == nil {
		err = p.files.Write("mcp.json", []byte("{\n  \"mcpServers\": {}\n}\n"))
	}
	p.configMu.Unlock()
	if err != nil {
		return SettingsView{}, err
	}
	p.refreshGlobal(ctx)
	return p.ReadSettings("")
}

func (p *Provider) refreshGlobal(ctx context.Context) {
	p.invalidateAll()
	_, _ = p.state(ctx, "")
}
