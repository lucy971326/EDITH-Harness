package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"harness/kernel/approvals"
	"harness/kernel/llm"
	"harness/kernel/permissions"
	"harness/kernel/persist"
	"harness/kernel/session"
	"harness/kernel/tools"
)

// 守住两道边界：项目连接前确认，以及每次工具转发前审批。
func TestProvider_projectTrustAndToolApproval(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "approval-test", Version: "v1"}, nil)
	var calls atomic.Int32
	server.AddTool(&sdk.Tool{Name: "ping", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			calls.Add(1)
			return &sdk.CallToolResult{}, nil
		})
	var connections atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server },
			&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}).ServeHTTP(w, r)
	}))
	defer remote.Close()
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, ".mcp.json")
	endpoint := remote.URL + "/mcp"
	config := `{"mcpServers":{"demo":{"type":"http","url":` + strconv.Quote(endpoint) + `}}}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := files.Write("config.yaml", []byte("jev: {}\n")); err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.Open(files, &llm.Client{}, &session.Store{})
	if err != nil {
		t.Fatal(err)
	}
	defer approval.Close()
	provider, err := newProvider(t.Context(), configFile{}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	provider.approvals = approval
	defer provider.Close()
	registry := tools.NewRegistry()
	if err := registry.RegisterProvider(provider); err != nil {
		t.Fatal(err)
	}
	access := tools.Access{Mode: permissions.AskForApproval, SessionID: "session", RunID: "run"}
	readOnly, err := registry.Prepare(t.Context(), workspace, nil, tools.Access{Mode: permissions.ReadOnly})
	if err != nil || len(readOnly.Names) != 0 || connections.Load() != 0 {
		t.Fatalf("read-only discovery = %#v, %v, connections=%d", readOnly, err, connections.Load())
	}

	type preparation struct {
		tools tools.Prepared
		err   error
	}
	prepare := func() <-chan preparation {
		result := make(chan preparation, 1)
		go func() {
			prepared, err := registry.Prepare(t.Context(), workspace, nil, access)
			result <- preparation{prepared, err}
		}()
		return result
	}
	waitPending := func(kind string) approvals.Pending {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			snapshot, updates, stop := approval.Subscribe()
			if len(snapshot) > 0 {
				stop()
				if snapshot[0].MCP == nil || snapshot[0].MCP.Kind != kind {
					t.Fatalf("pending = %#v, want %s", snapshot, kind)
				}
				return snapshot[0]
			}
			select {
			case snapshot = <-updates:
				stop()
				if len(snapshot) > 0 {
					if snapshot[0].MCP == nil || snapshot[0].MCP.Kind != kind {
						t.Fatalf("pending = %#v, want %s", snapshot, kind)
					}
					return snapshot[0]
				}
			case <-deadline:
				stop()
				t.Fatal("approval not published")
			}
		}
	}
	first := prepare()
	pending := waitPending("config")
	if len(pending.MCP.Servers) != 1 || pending.MCP.Servers[0].Target != endpoint {
		t.Fatalf("approval does not show the full endpoint: %#v", pending.MCP.Servers)
	}
	if connections.Load() != 0 {
		t.Fatal("project server connected before confirmation")
	}
	if err := approval.Respond(pending.ID, permissions.Decision{Approved: false}); err != nil {
		t.Fatal(err)
	}
	if got := <-first; got.err != nil || len(got.tools.Names) != 0 || connections.Load() != 0 {
		t.Fatalf("denied project discovery = %#v, connections=%d", got, connections.Load())
	}
	access.RunID = "next-run"
	second := prepare()
	pending = waitPending("config")
	if err := approval.Respond(pending.ID, permissions.Decision{Approved: true}); err != nil {
		t.Fatal(err)
	}
	ready := <-second
	if ready.err != nil {
		t.Fatal(ready.err)
	}
	prepared := ready.tools
	if len(prepared.Names) != 1 || prepared.Names[0] != "mcp__demo__ping" {
		t.Fatalf("approved discovery = %#v", prepared)
	}

	call := tools.Call{Name: "mcp__demo__ping", Arguments: json.RawMessage(`{}`), Workspace: workspace,
		Mode: permissions.AskForApproval, Reviewer: permissions.HumanReviewer, Allow: prepared.Names,
		SessionID: "session", RunID: "next-run", ToolCallID: "call"}
	result := make(chan tools.Result, 1)
	go func() {
		value, _ := registry.Call(t.Context(), call)
		result <- value
	}()
	pending = waitPending("call")
	if calls.Load() != 0 {
		t.Fatal("MCP tool called before approval")
	}
	if err := approval.Respond(pending.ID, permissions.Decision{Approved: false}); err != nil {
		t.Fatal(err)
	}
	if got := <-result; !got.IsError || calls.Load() != 0 {
		t.Fatalf("denied call = %#v, calls=%d", got, calls.Load())
	}
	go func() {
		value, _ := registry.Call(t.Context(), call)
		result <- value
	}()
	pending = waitPending("call")
	if err := approval.Respond(pending.ID, permissions.Decision{Approved: true}); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got.IsError || calls.Load() != 1 {
		t.Fatalf("approved call = %#v, calls=%d", got, calls.Load())
	}
	call.Mode = permissions.ReadOnly
	got, err := registry.Call(t.Context(), call)
	if err != nil || !got.IsError || calls.Load() != 1 {
		t.Fatalf("read-only call = %#v, %v, calls=%d", got, err, calls.Load())
	}
	call.Mode = permissions.FullAccess
	got, err = registry.Call(t.Context(), call)
	if err != nil || got.IsError || calls.Load() != 2 {
		t.Fatalf("full-access call = %#v, %v, calls=%d", got, err, calls.Load())
	}
	restarted, err := newProvider(t.Context(), configFile{}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	restarted.approvals = approval
	defer restarted.Close()
	checkCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	snapshot, err := restarted.Snapshot(tools.WithAccess(checkCtx, tools.Access{
		Mode: permissions.AskForApproval, SessionID: "session", RunID: "restart",
	}), workspace)
	if err != nil || len(snapshot.Definitions) != 1 {
		t.Fatalf("remembered project confirmation = %#v, %v", snapshot, err)
	}
	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = registry.Call(t.Context(), call)
	if err != nil || !got.IsError || calls.Load() != 2 {
		t.Fatalf("changed configuration call = %#v, %v, calls=%d", got, err, calls.Load())
	}
}

func TestProvider_rejectsRetargetedWorkspace(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, path := range []string{first, second} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	workspace := filepath.Join(root, "current")
	if err := os.Symlink(first, workspace); err != nil {
		t.Skipf("workspace symlinks are unavailable: %v", err)
	}
	provider, err := newProvider(t.Context(), configFile{}, root)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	access := tools.WithAccess(t.Context(), tools.Access{Mode: permissions.FullAccess})
	if _, err := provider.Snapshot(access, workspace); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(workspace); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Snapshot(access, workspace); err == nil || !strings.Contains(err.Error(), "project path changed") {
		t.Fatalf("retargeted workspace reused old MCP connection: %v", err)
	}
}

func TestProvider_retriesFailedWorkspaceAndKeepsSuccessfulSnapshot(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "retry-test", Version: "v1"}, nil)
	server.AddTool(&sdk.Tool{
		Name: "ping", Description: "Ping.", InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{}, nil
	})
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	var available atomic.Bool
	var requests atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if !available.Load() {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	workspace := t.TempDir()
	config := `{"mcpServers":{"demo":{"type":"http","url":` + strconv.Quote(httpServer.URL) + `}}}`
	err := os.WriteFile(filepath.Join(workspace, ".mcp.json"), []byte(config), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	p, err := newProvider(t.Context(), configFile{}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	accessCtx := tools.WithAccess(t.Context(), tools.Access{Mode: permissions.FullAccess})
	_, err = p.Snapshot(accessCtx, workspace)
	if err == nil {
		t.Fatal("first discovery should fail")
	}
	available.Store(true)
	snapshot, err := p.Snapshot(accessCtx, workspace)
	if err != nil {
		t.Fatalf("discovery after server recovery: %v", err)
	}
	if len(snapshot.Definitions) != 1 || snapshot.Definitions[0].Name != "mcp__demo__ping" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	count := requests.Load()
	available.Store(false)
	_, err = p.Snapshot(accessCtx, workspace)
	if err != nil {
		t.Fatalf("successful snapshot should remain cached: %v", err)
	}
	if requests.Load() != count {
		t.Fatal("cached snapshot triggered rediscovery")
	}
}

func TestConfig_isStrictAndExpandsEnvironment(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "secret")
	config := configFile{MCPServers: map[string]serverConfig{
		"remote": {
			Type:    "streamable-http",
			URL:     "https://example.test/${MCP_TEST_TOKEN}",
			Headers: map[string]string{"Authorization": "Bearer ${MISSING:-fallback}"},
		},
	}}
	specs, err := normalizeServers(config, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if specs[0].Transport != transportHTTP || specs[0].URL != "https://example.test/secret" {
		t.Fatalf("spec = %#v", specs[0])
	}
	if specs[0].Headers["Authorization"] != "Bearer fallback" {
		t.Fatalf("headers = %#v", specs[0].Headers)
	}

	path := filepath.Join(t.TempDir(), "mcp.json")
	err = os.WriteFile(path, []byte(`{"mcpServers":{},"servers":{}}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = readConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field", err)
	}
}

func TestProvider_userToolsJoinPrepareWithoutSelection(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "user-test", Version: "v1"}, nil)
	server.AddTool(&sdk.Tool{
		Name: "search", Description: "Search.", InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "hits"}}}, nil
	})
	httpServer := httptest.NewServer(sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	))
	defer httpServer.Close()

	userConfig := filepath.Join(t.TempDir(), "mcp.json")
	err := os.WriteFile(userConfig, []byte(`{"mcpServers":{"tavily":{"type":"http","url":`+strconv.Quote(httpServer.URL)+`}}}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	userSettings, _, err := readConfig(userConfig)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := newProvider(context.Background(), userSettings, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	registry := tools.NewRegistry()
	err = registry.RegisterProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.List()) != 0 {
		t.Fatalf("List() = %#v", registry.List())
	}
	prepared, err := registry.Prepare(context.Background(), "", nil, tools.Access{Mode: permissions.FullAccess})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Names) != 1 || prepared.Names[0] != "mcp__tavily__search" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestProvider_projectToolsAreAutomaticAndCallable(t *testing.T) {
	server := sdk.NewServer(
		&sdk.Implementation{Name: "test", Version: "v1"},
		&sdk.ServerOptions{Instructions: "Read before changing data.", PageSize: 1},
	)
	server.AddTool(&sdk.Tool{
		Name:        "greet",
		Description: "Greet a person.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
	}, func(_ context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		var input struct {
			Name string `json:"name"`
		}
		err := json.Unmarshal(request.Params.Arguments, &input)
		if err != nil {
			return nil, err
		}
		return &sdk.CallToolResult{
			Content:           []sdk.Content{&sdk.TextContent{Text: "hello " + input.Name}},
			StructuredContent: map[string]any{"name": input.Name},
		}, nil
	})
	server.AddTool(&sdk.Tool{
		Name:        "hidden",
		Description: "Hidden tool.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{}, nil
	})
	httpServer := httptest.NewServer(sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	))
	defer httpServer.Close()

	workspace := t.TempDir()
	config := `{"mcpServers":{"demo":{"type":"http","url":` + strconv.Quote(httpServer.URL) + `,"includeTools":["greet"]}}}`
	err := os.WriteFile(filepath.Join(workspace, ".mcp.json"), []byte(config), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := newProvider(context.Background(), configFile{}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	registry := tools.NewRegistry()
	err = registry.RegisterProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := registry.Prepare(context.Background(), workspace, nil, tools.Access{Mode: permissions.FullAccess})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Names) != 1 || prepared.Names[0] != "mcp__demo__greet" || len(prepared.Instructions) != 1 {
		t.Fatalf("prepared = %#v", prepared)
	}
	result, err := registry.Call(context.Background(), tools.Call{
		Name:      "mcp__demo__greet",
		Arguments: json.RawMessage(`{"name":"Lucy"}`),
		Workspace: workspace,
		Mode:      permissions.FullAccess,
		Allow:     prepared.Names,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, "hello Lucy") || !strings.Contains(result.Content, `"name":"Lucy"`) {
		t.Fatalf("result = %#v", result)
	}
}
