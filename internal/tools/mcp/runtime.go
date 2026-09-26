package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"harness/internal/approvals"
	"harness/internal/permissions"
	"harness/internal/persist"
	"harness/internal/tools"
)

type serverStatus struct {
	state   string
	message string
	tools   []string
}

// 活对象。一个版本的 MCP 连接、工具路由和引用计数。
type workspaceState struct {
	once      sync.Once
	plan      sourcePlan
	snapshot  tools.Snapshot
	routes    map[string]*serverConnection
	owned     []*serverConnection
	status    map[string]serverStatus
	err       error
	retryNext bool
	built     bool
	refs      int
	retired   bool
	closed    bool
}

// 活对象。按 Run 固定 MCP 配置和连接，配置写入与连接状态归此领域管理。
type Provider struct {
	files        *persist.Files
	launchDir    string
	userOverride *configFile // 仅供包内测试构造。
	approvals    *approvals.Service

	configMu sync.Mutex // 保护后端对 mcp.json 的版本比较与写入。
	mu       sync.Mutex // 保护 current、runs、连接引用与关闭状态。
	current  map[string]*workspaceState
	runs     map[string]*workspaceState
	closed   bool
	wg       sync.WaitGroup
}

func newProvider(ctx context.Context, config configFile, launchDir string) (*Provider, error) {
	p := &Provider{launchDir: launchDir, userOverride: &config,
		current: make(map[string]*workspaceState), runs: make(map[string]*workspaceState)}
	_, err := p.state(ctx, "")
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Provider) Name() string { return "mcp" }

func runKey(sessionID, runID string) string { return sessionID + ":" + runID }

// Snapshot 在本轮第一次发现时固定连接版本；之后的定义读取不重新查文件。
func (p *Provider) Snapshot(ctx context.Context, workspace string) (tools.Snapshot, error) {
	if tools.AccessFromContext(ctx).Mode == permissions.ReadOnly {
		return tools.Snapshot{}, nil
	}
	state, err := p.state(ctx, workspace)
	if err != nil {
		return tools.Snapshot{}, err
	}
	return copySnapshot(state.snapshot), nil
}

func (p *Provider) state(ctx context.Context, workspace string) (*workspaceState, error) {
	access := tools.AccessFromContext(ctx)
	key := ""
	if access.RunID != "" {
		key = runKey(access.SessionID, access.RunID)
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("mcp: provider is closed")
	}
	if key != "" {
		if pinned := p.runs[key]; pinned != nil {
			p.mu.Unlock()
			pinned.once.Do(func() { p.build(ctx, workspace, pinned) })
			return pinned, pinned.err
		}
	}
	p.mu.Unlock()

	plan, err := p.plan(workspace)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("mcp: provider is closed")
	}
	if key != "" {
		if pinned := p.runs[key]; pinned != nil {
			p.mu.Unlock()
			pinned.once.Do(func() { p.build(ctx, workspace, pinned) })
			return pinned, pinned.err
		}
	}
	state := p.current[workspace]
	var closing []*serverConnection
	if state == nil || state.plan.version != plan.version || (state.built && state.retryNext) {
		if state != nil {
			state.retired = true
			closing = p.collectLocked(state)
		}
		state = &workspaceState{plan: plan}
		p.current[workspace] = state
	}
	state.refs++ // 在连接和可能的信任审批完成前，也不能释放此版本。
	if key != "" {
		p.runs[key] = state
	}
	p.wg.Add(1)
	p.mu.Unlock()
	closeConnections(closing)
	defer p.wg.Done()
	state.once.Do(func() { p.build(ctx, workspace, state) })

	p.mu.Lock()
	err = state.err
	if key == "" || err != nil {
		if key != "" {
			delete(p.runs, key)
		}
		state.refs--
		closing = p.collectLocked(state)
	} else {
		closing = nil
	}
	p.mu.Unlock()
	closeConnections(closing)
	return state, err
}

func (p *Provider) build(ctx context.Context, workspace string, state *workspaceState) {
	plan := state.plan
	entries := plan.entries
	var buildErr error
	retryNext := false
	if workspace != "" && plan.projectError == "" {
		projectEntries := make([]plannedServer, 0)
		for _, entry := range entries {
			if entry.project {
				projectEntries = append(projectEntries, entry)
			}
		}
		if len(projectEntries) > 0 && p.approvals != nil {
			realPath, err := filepath.EvalSymlinks(workspace)
			if err != nil {
				buildErr = fmt.Errorf("mcp: project path unavailable")
			}
			if buildErr == nil {
				request := approvals.MCPRequest{Kind: "config", Workspace: realPath, Source: plan.projectSource}
				for _, entry := range projectEntries {
					target := "HTTP · 地址已设置"
					if entry.config.Type == transportStdio {
						target = fmt.Sprintf("STDIO · 命令已设置 · %d 个参数", len(entry.config.Args))
					}
					request.Servers = append(request.Servers, approvals.MCPServer{Name: entry.name, Target: target})
				}
				access := tools.AccessFromContext(ctx)
				buildErr = p.approvals.ConfirmMCPConfig(ctx, approvals.Identity{SessionID: access.SessionID, RunID: access.RunID}, plan.projectDigest, request)
				if errors.Is(buildErr, approvals.ErrMCPConfigDenied) {
					entries = nil
					for _, entry := range plan.entries {
						if !entry.project {
							entries = append(entries, entry)
						}
					}
					buildErr = nil
					retryNext = true
				}
			}
			if buildErr == nil && !retryNext {
				current, err := p.plan(workspace)
				if err != nil || current.version != plan.version {
					buildErr = fmt.Errorf("mcp: project configuration changed during confirmation")
					retryNext = true
				}
			}
		}
	}
	connections := make([]*serverConnection, 0)
	status := make(map[string]serverStatus, len(entries))
	if buildErr == nil {
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				buildErr = err
				break
			}
			if !entry.config.isEnabled() {
				status[entry.name] = serverStatus{state: "disabled"}
				continue
			}
			if entry.err != "" {
				status[entry.name] = serverStatus{state: "invalid", message: entry.err}
				continue
			}
			if entry.spec == nil {
				continue
			}
			connectCtx, cancel := context.WithTimeout(ctx, startupTimeout)
			connection, err := connectServer(connectCtx, *entry.spec)
			cancel()
			if err != nil {
				status[entry.name] = serverStatus{state: "failed", message: safeConnectionError(err)}
				continue
			}
			connections = append(connections, connection)
			tools := make([]string, 0, len(connection.definitions))
			for _, definition := range connection.definitions {
				tools = append(tools, definition.Name)
			}
			status[entry.name] = serverStatus{state: "connected", tools: tools}
		}
	}
	if buildErr != nil {
		closeConnections(connections)
		connections = nil
	}
	if buildErr != nil || workspace != "" {
		for _, item := range status {
			if item.state == "failed" {
				retryNext = true
				break
			}
		}
	}
	if buildErr != nil {
		retryNext = true
	}
	snapshot, routes := snapshotFromConnections(connections)
	p.mu.Lock()
	state.owned, state.snapshot, state.routes, state.status = connections, snapshot, routes, status
	state.err, state.retryNext, state.built = buildErr, retryNext, true
	p.mu.Unlock()
}

func safeConnectionError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "连接超时"
	}
	if errors.Is(err, context.Canceled) {
		return "连接已取消"
	}
	return "连接或工具发现失败"
}

// Call 使用本轮固定路由，不因保存、关闭或项目配置改变重新查询 Provider。
func (p *Provider) Call(ctx context.Context, call tools.Call) (tools.Result, error) {
	if call.Mode == permissions.ReadOnly {
		return tools.Result{}, fmt.Errorf("mcp: tool call is not allowed in this permission mode")
	}
	if p.approvals != nil {
		switch call.Mode {
		case permissions.AskForApproval:
			if call.Reviewer != permissions.HumanReviewer {
				return tools.Result{}, fmt.Errorf("mcp: human approval is required")
			}
		case permissions.ApproveForMe:
			if call.Reviewer != permissions.ModelReviewer {
				return tools.Result{}, fmt.Errorf("mcp: model approval is required")
			}
		case permissions.FullAccess:
		default:
			return tools.Result{}, fmt.Errorf("mcp: tool call has no valid permission mode")
		}
	}
	p.mu.Lock()
	state := p.runs[runKey(call.SessionID, call.RunID)]
	p.mu.Unlock()
	if state == nil {
		if call.RunID != "" {
			return tools.Result{}, fmt.Errorf("mcp: run connection is no longer available")
		}
		var err error
		state, err = p.state(tools.WithAccess(ctx, tools.Access{Mode: call.Mode}), call.Workspace)
		if err != nil {
			return tools.Result{}, err
		}
	}
	connection := state.routes[call.Name]
	if connection == nil {
		return tools.Result{}, fmt.Errorf("mcp: tool %q is not in run snapshot", call.Name)
	}
	remoteName := connection.remoteNames[call.Name]
	var arguments map[string]any
	if err := json.Unmarshal(call.Arguments, &arguments); err != nil {
		return tools.Result{}, fmt.Errorf("mcp: decode arguments for %q: %w", call.Name, err)
	}
	if p.approvals != nil && call.Mode != permissions.FullAccess {
		approval := approvals.MCPRequest{Kind: "call", Workspace: call.Workspace, Server: connection.name, Tool: remoteName, Arguments: append(json.RawMessage(nil), call.Arguments...)}
		err := p.approvals.AuthorizeMCPCall(ctx, approvals.Identity{SessionID: call.SessionID, RunID: call.RunID, ToolCallID: call.ToolCallID}, call.Reviewer, approval)
		if err != nil {
			return tools.Result{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()
	result, err := connection.session.CallTool(callCtx, &sdk.CallToolParams{Name: remoteName, Arguments: arguments})
	if err != nil {
		return tools.Result{}, fmt.Errorf("mcp: call %s/%s failed", connection.name, remoteName)
	}
	return renderResult(result)
}

// ReleaseRun 释放本轮引用；被新配置替换的旧连接在最后一轮结束后关闭。
func (p *Provider) ReleaseRun(sessionID, id string) {
	key := runKey(sessionID, id)
	p.mu.Lock()
	state := p.runs[key]
	if state != nil {
		delete(p.runs, key)
		state.refs--
	}
	closing := p.collectLocked(state)
	p.mu.Unlock()
	closeConnections(closing)
}

func (p *Provider) invalidate(workspace string) {
	p.mu.Lock()
	state := p.current[workspace]
	delete(p.current, workspace)
	if state != nil {
		state.retired = true
	}
	closing := p.collectLocked(state)
	p.mu.Unlock()
	closeConnections(closing)
}

func (p *Provider) invalidateAll() {
	p.mu.Lock()
	var closing []*serverConnection
	for workspace, state := range p.current {
		delete(p.current, workspace)
		state.retired = true
		closing = append(closing, p.collectLocked(state)...)
	}
	p.mu.Unlock()
	_ = closeConnections(closing)
}

func (p *Provider) collectLocked(state *workspaceState) []*serverConnection {
	if state == nil || !state.retired || !state.built || state.refs != 0 || state.closed {
		return nil
	}
	state.closed = true
	return state.owned
}

func closeConnections(connections []*serverConnection) error {
	var errs []error
	for _, connection := range connections {
		err := connection.session.Close()
		if err != nil && !connection.stdio {
			errs = append(errs, fmt.Errorf("close server %q: %w", connection.name, err))
		}
	}
	return errors.Join(errs...)
}

// Close 等待构建结束并关闭全部版本。
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
	seen := make(map[*workspaceState]struct{})
	for _, state := range p.current {
		seen[state] = struct{}{}
	}
	for _, state := range p.runs {
		seen[state] = struct{}{}
	}
	var closing []*serverConnection
	for state := range seen {
		if !state.closed {
			state.closed = true
			closing = append(closing, state.owned...)
		}
	}
	p.current = nil
	p.runs = nil
	p.mu.Unlock()
	return closeConnections(closing)
}
