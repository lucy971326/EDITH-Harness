package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"harness/kernel/tools"
)

const (
	toolTimeout         = 10 * time.Minute
	maxInstructionBytes = 2 * 1024
)

// 活对象。MCP Server 的一条已连接会话及其工具路由。
type serverConnection struct {
	name         string
	stdio        bool
	session      *sdk.ClientSession
	definitions  []tools.Definition
	remoteNames  map[string]string
	instructions string
}

// 活对象。一个工作区固定使用的 MCP 工具快照。
type workspaceState struct {
	once     sync.Once
	snapshot tools.Snapshot
	routes   map[string]*serverConnection
	owned    []*serverConnection
	err      error
}

// 活对象。按工作区发现并调用 MCP Tool 的动态来源。
type Provider struct {
	user *workspaceState

	mu         sync.Mutex
	workspaces map[string]*workspaceState
	closed     bool
	wg         sync.WaitGroup
}

func newProvider(ctx context.Context, userConfigPath string, launchDir string) (*Provider, error) {
	config, _, err := readConfig(userConfigPath)
	if err != nil {
		return nil, err
	}
	specs, err := normalizeServers(config, launchDir)
	if err != nil {
		return nil, err
	}
	connections, err := connectAll(ctx, specs)
	if err != nil {
		return nil, err
	}
	provider := &Provider{
		workspaces: make(map[string]*workspaceState),
	}
	provider.user = stateFromConnections(connections, false)
	provider.user.owned = connections
	return provider, nil
}

func (p *Provider) Name() string { return "mcp" }

// Choices 返回用户级 MCP Tool，供 Agent 设置选择。
func (p *Provider) Choices() []tools.Definition {
	return append([]tools.Definition(nil), p.user.snapshot.Definitions...)
}

// Snapshot 返回用户配置和当前项目配置合成的稳定快照。
func (p *Provider) Snapshot(ctx context.Context, workspace string) (tools.Snapshot, error) {
	state, err := p.state(ctx, workspace)
	if err != nil {
		return tools.Snapshot{}, err
	}
	return copySnapshot(state.snapshot), nil
}

// Call 把普通 Harness Tool 调用转给所属 MCP Server。
func (p *Provider) Call(ctx context.Context, call tools.Call) (tools.Result, error) {
	state, err := p.state(ctx, call.Workspace)
	if err != nil {
		return tools.Result{}, err
	}
	connection := state.routes[call.Name]
	if connection == nil {
		return tools.Result{}, fmt.Errorf("mcp: tool %q is not in workspace snapshot", call.Name)
	}
	remoteName := connection.remoteNames[call.Name]
	var arguments map[string]any
	err = json.Unmarshal(call.Arguments, &arguments)
	if err != nil {
		return tools.Result{}, fmt.Errorf("mcp: decode arguments for %q: %w", call.Name, err)
	}
	callCtx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()
	result, err := connection.session.CallTool(callCtx, &sdk.CallToolParams{Name: remoteName, Arguments: arguments})
	if err != nil {
		return tools.Result{}, fmt.Errorf("mcp: call %s/%s: %w", connection.name, remoteName, err)
	}
	return renderResult(result)
}

// Close 停止全部用户级和工作区 MCP 会话。
func (p *Provider) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()
	p.wg.Wait()

	p.mu.Lock()
	connections := append([]*serverConnection(nil), p.user.owned...)
	for _, state := range p.workspaces {
		connections = append(connections, state.owned...)
	}
	p.workspaces = make(map[string]*workspaceState)
	p.mu.Unlock()

	var errs []error
	for _, connection := range connections {
		err := connection.session.Close()
		if err != nil && !connection.stdio {
			errs = append(errs, fmt.Errorf("close server %q: %w", connection.name, err))
		}
	}
	return errors.Join(errs...)
}

func (p *Provider) state(ctx context.Context, workspace string) (*workspaceState, error) {
	if workspace == "" {
		p.mu.Lock()
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return nil, fmt.Errorf("mcp: provider is closed")
		}
		return p.user, nil
	}
	key, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("mcp: normalize workspace %q: %w", workspace, err)
	}
	key = filepath.Clean(key)

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("mcp: provider is closed")
	}
	state := p.workspaces[key]
	if state == nil {
		state = &workspaceState{}
		p.workspaces[key] = state
	}
	p.wg.Add(1)
	p.mu.Unlock()
	defer p.wg.Done()

	state.once.Do(func() {
		state.err = p.loadWorkspace(ctx, key, state)
	})
	if state.err != nil {
		return nil, state.err
	}
	return state, nil
}

func (p *Provider) loadWorkspace(ctx context.Context, workspace string, state *workspaceState) error {
	root, _, err := readConfig(filepath.Join(workspace, ".mcp.json"))
	if err != nil {
		return err
	}
	nested, _, err := readConfig(filepath.Join(workspace, ".harness", "mcp.json"))
	if err != nil {
		return err
	}
	merged := configFile{MCPServers: make(map[string]serverConfig)}
	for name, config := range root.MCPServers {
		merged.MCPServers[name] = config
	}
	for name, config := range nested.MCPServers {
		merged.MCPServers[name] = config
	}
	specs, err := normalizeServers(merged, workspace)
	if err != nil {
		return err
	}
	loadCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	connections, err := connectAll(loadCtx, specs)
	if err != nil {
		return err
	}
	state.owned = connections
	projectNames := make(map[string]struct{}, len(connections))
	for _, connection := range connections {
		projectNames[connection.name] = struct{}{}
	}
	combined := make([]*serverConnection, 0, len(p.user.owned)+len(connections))
	for _, connection := range p.user.owned {
		if _, overridden := projectNames[connection.name]; !overridden {
			combined = append(combined, connection)
		}
	}
	combined = append(combined, connections...)
	state.snapshot, state.routes = snapshotFromConnections(combined, projectNames)
	return nil
}

func connectAll(ctx context.Context, specs []serverSpec) ([]*serverConnection, error) {
	connections := make([]*serverConnection, 0, len(specs))
	for _, spec := range specs {
		connection, err := connectServer(ctx, spec)
		if err != nil {
			for _, opened := range connections {
				_ = opened.session.Close()
			}
			return nil, err
		}
		connections = append(connections, connection)
	}
	return connections, nil
}

func connectServer(ctx context.Context, spec serverSpec) (*serverConnection, error) {
	var transport sdk.Transport
	if spec.Transport == transportStdio {
		command := exec.Command(spec.Command, spec.Args...)
		command.Dir = spec.CWD
		command.Env = mergedEnvironment(spec.Env)
		transport = &sdk.CommandTransport{Command: command}
	} else {
		client := &http.Client{Transport: headerTransport{base: http.DefaultTransport, headers: spec.Headers}}
		transport = &sdk.StreamableClientTransport{
			Endpoint:             spec.URL,
			HTTPClient:           client,
			DisableStandaloneSSE: true,
		}
	}
	client := sdk.NewClient(
		&sdk.Implementation{Name: "harness", Version: "v1"},
		&sdk.ClientOptions{Capabilities: &sdk.ClientCapabilities{}},
	)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: connect server %q: %w", spec.Name, err)
	}
	connection, err := discoverServer(ctx, spec, session)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	return connection, nil
}

func discoverServer(ctx context.Context, spec serverSpec, session *sdk.ClientSession) (*serverConnection, error) {
	allNames := make(map[string]struct{})
	remoteTools := make([]*sdk.Tool, 0)
	for remote, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("mcp: list tools from %q: %w", spec.Name, err)
		}
		if remote == nil || remote.Name == "" {
			return nil, fmt.Errorf("mcp: server %q returned tool with empty name", spec.Name)
		}
		if _, duplicate := allNames[remote.Name]; duplicate {
			return nil, fmt.Errorf("mcp: server %q returned duplicate tool %q", spec.Name, remote.Name)
		}
		allNames[remote.Name] = struct{}{}
		remoteTools = append(remoteTools, remote)
	}
	err := validateFilters(spec, allNames)
	if err != nil {
		return nil, err
	}
	definitions := make([]tools.Definition, 0, len(remoteTools))
	routes := make(map[string]string, len(remoteTools))
	for _, remote := range remoteTools {
		if !included(spec, remote.Name) {
			continue
		}
		schema, err := json.Marshal(remote.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("mcp: marshal schema for %s/%s: %w", spec.Name, remote.Name, err)
		}
		name := "mcp__" + spec.Name + "__" + remote.Name
		if len(name) > 128 {
			return nil, fmt.Errorf("mcp: wrapped tool name %q exceeds 128 bytes", name)
		}
		description := strings.TrimSpace(remote.Description)
		if description == "" {
			description = fmt.Sprintf("MCP tool %s from server %s.", remote.Name, spec.Name)
		}
		definitions = append(definitions, tools.Definition{Name: name, Description: description, InputSchema: schema})
		routes[name] = remote.Name
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	instructions := ""
	if initialized := session.InitializeResult(); initialized != nil {
		instructions = truncateInstruction(strings.TrimSpace(initialized.Instructions))
	}
	return &serverConnection{
		name:         spec.Name,
		stdio:        spec.Transport == transportStdio,
		session:      session,
		definitions:  definitions,
		remoteNames:  routes,
		instructions: instructions,
	}, nil
}

func stateFromConnections(connections []*serverConnection, automatic bool) *workspaceState {
	automaticNames := make(map[string]struct{})
	if automatic {
		for _, connection := range connections {
			automaticNames[connection.name] = struct{}{}
		}
	}
	snapshot, routes := snapshotFromConnections(connections, automaticNames)
	return &workspaceState{snapshot: snapshot, routes: routes}
}

func snapshotFromConnections(connections []*serverConnection, automaticServers map[string]struct{}) (tools.Snapshot, map[string]*serverConnection) {
	var snapshot tools.Snapshot
	routes := make(map[string]*serverConnection)
	for _, connection := range connections {
		toolNames := make([]string, 0, len(connection.definitions))
		for _, definition := range connection.definitions {
			snapshot.Definitions = append(snapshot.Definitions, definition)
			toolNames = append(toolNames, definition.Name)
			routes[definition.Name] = connection
			if _, automatic := automaticServers[connection.name]; automatic {
				snapshot.Automatic = append(snapshot.Automatic, definition.Name)
			}
		}
		if connection.instructions != "" && len(toolNames) > 0 {
			snapshot.Instructions = append(snapshot.Instructions, tools.Instruction{
				Source:    connection.name,
				ToolNames: toolNames,
				Text:      connection.instructions,
			})
		}
	}
	sort.Slice(snapshot.Definitions, func(i, j int) bool { return snapshot.Definitions[i].Name < snapshot.Definitions[j].Name })
	sort.Strings(snapshot.Automatic)
	return snapshot, routes
}

func copySnapshot(snapshot tools.Snapshot) tools.Snapshot {
	out := tools.Snapshot{
		Definitions: append([]tools.Definition(nil), snapshot.Definitions...),
		Automatic:   append([]string(nil), snapshot.Automatic...),
	}
	for _, instruction := range snapshot.Instructions {
		instruction.ToolNames = append([]string(nil), instruction.ToolNames...)
		out.Instructions = append(out.Instructions, instruction)
	}
	return out
}

func validateFilters(spec serverSpec, available map[string]struct{}) error {
	if spec.IncludeTools != nil {
		for _, name := range *spec.IncludeTools {
			if _, ok := available[name]; !ok {
				return fmt.Errorf("mcp: server %q includes unknown tool %q", spec.Name, name)
			}
		}
	}
	for _, name := range spec.ExcludeTools {
		if _, ok := available[name]; !ok {
			return fmt.Errorf("mcp: server %q excludes unknown tool %q", spec.Name, name)
		}
	}
	return nil
}

func included(spec serverSpec, name string) bool {
	if spec.IncludeTools != nil {
		found := false
		for _, allowed := range *spec.IncludeTools {
			found = found || allowed == name
		}
		if !found {
			return false
		}
	}
	for _, excluded := range spec.ExcludeTools {
		if excluded == name {
			return false
		}
	}
	return true
}

func mergedEnvironment(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, pair := range os.Environ() {
		if key, value, ok := strings.Cut(pair, "="); ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func truncateInstruction(text string) string {
	if len(text) <= maxInstructionBytes {
		return text
	}
	text = text[:maxInstructionBytes]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text
}

// 活对象。给 Streamable HTTP 请求补充静态 Header 的 transport。
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	for name, value := range t.headers {
		if cloned.Header.Get(name) == "" {
			cloned.Header.Set(name, value)
		}
	}
	return t.base.RoundTrip(cloned)
}
