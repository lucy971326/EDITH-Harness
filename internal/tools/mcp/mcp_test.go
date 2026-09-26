package mcp

import (
	"context"
	"encoding/json"
	"errors"
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

	"harness/internal/approvals"
	"harness/internal/llm"
	"harness/internal/permissions"
	"harness/internal/persist"
	"harness/internal/session"
	"harness/internal/tools"
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
	settingsView, err := provider.ReadSettings(workspace)
	if err != nil || len(settingsView.Project) != 1 || connections.Load() != 0 {
		t.Fatalf("read-only settings connected project server: %#v, %v", settingsView, err)
	}
	registry := tools.NewRegistry()
	if err := registry.RegisterProvider(provider); err != nil {
		t.Fatal(err)
	}
	access := tools.Access{Mode: permissions.AskForApproval, SessionID: "session", RunID: "run"}
	readOnly, err := registry.Prepare(t.Context(), workspace, nil, tools.Access{Mode: permissions.ReadOnly})
	if err != nil || len(readOnly.Names) != 0 || connections.Load() != 0 {
		t.Fatalf("read-only discovery = %#v, %v, connections=%d", readOnly, err, connections.Load())
	}
	plan, err := provider.plan(workspace)
	if err != nil || plan.projectError != "" || len(plan.entries) != 1 || !plan.entries[0].project {
		t.Fatalf("project plan = %#v, %v", plan, err)
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
	if len(pending.MCP.Servers) != 1 || strings.Contains(pending.MCP.Servers[0].Target, endpoint) {
		t.Fatalf("approval exposed the endpoint: %#v", pending.MCP.Servers)
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
	if err != nil || got.IsError || calls.Load() != 3 {
		t.Fatalf("old run lost its approved connection: %#v, %v, calls=%d", got, err, calls.Load())
	}
	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{"demo":{"type":"http","url":`+strconv.Quote(endpoint)+`,"includeTools":["ping"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	access.RunID = "changed-run"
	third := prepare()
	pending = waitPending("config")
	if connections.Load() == 0 {
		t.Fatal("old connection unexpectedly closed")
	}
	if err := approval.Respond(pending.ID, permissions.Decision{Approved: true}); err != nil {
		t.Fatal(err)
	}
	if got := <-third; got.err != nil || len(got.tools.Names) != 1 {
		t.Fatalf("changed project config did not reapprove: %#v", got)
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
	before, err := provider.plan(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Snapshot(access, workspace); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(workspace); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, workspace); err != nil {
		t.Fatal(err)
	}
	after, err := provider.plan(workspace)
	if err != nil || before.version == after.version {
		t.Fatalf("retargeted workspace kept the old source version: %v", err)
	}
	if _, err := provider.Snapshot(access, workspace); err != nil {
		t.Fatalf("retargeted workspace could not be refreshed: %v", err)
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
	if err != nil {
		t.Fatalf("one failed server should be isolated: %v", err)
	}
	available.Store(true)
	p.invalidate(workspace)
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

func TestSettings_versionRepairAndSecretView(t *testing.T) {
	t.Setenv("MCP_TEST_SECRET", "not-for-the-browser")
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider, err := New(files, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	view, err := provider.ReadSettings("")
	if err != nil {
		t.Fatal(err)
	}
	protocol := "http"
	endpoint := "http://127.0.0.1:1/${MCP_TEST_SECRET}"
	view, err = provider.Save(t.Context(), SaveInput{Name: "demo", Revision: view.Revision,
		Create: true, Type: &protocol, URL: &endpoint})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "not-for-the-browser") || strings.Contains(string(encoded), endpoint) {
		t.Fatalf("settings view disclosed URL or expanded secret: %s", encoded)
	}
	if len(view.Global) != 1 || view.Global[0].Status != "failed" {
		t.Fatalf("saved configuration should report connection failure: %#v", view.Global)
	}
	stale := view.Revision
	if err := files.Write("mcp.json", []byte(`{"mcpServers":{}}`)); err != nil {
		t.Fatal(err)
	}
	_, err = provider.Delete(t.Context(), "demo", stale)
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("stale delete = %v", err)
	}
	if err := files.Write("mcp.json", []byte(`{broken`)); err != nil {
		t.Fatal(err)
	}
	broken, err := provider.ReadSettings("")
	if err != nil || broken.GlobalError == "" {
		t.Fatalf("broken config = %#v, %v", broken, err)
	}
	view, err = provider.ResetInvalid(t.Context(), broken.Revision)
	if err != nil || view.GlobalError != "" {
		t.Fatalf("reset = %#v, %v", view, err)
	}
	data, err := os.ReadDir(filepath.Dir(view.GlobalPath))
	if err != nil {
		t.Fatal(err)
	}
	backedUp := false
	for _, entry := range data {
		if strings.HasPrefix(entry.Name(), "mcp.json.backup-") {
			backedUp = true
		}
	}
	if !backedUp {
		t.Fatal("damaged config was not backed up")
	}
}

func TestProvider_projectDisabledNameDoesNotFallBack(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "global", Version: "v1"}, nil)
	server.AddTool(&sdk.Tool{Name: "ping", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		})
	remote := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer remote.Close()
	provider, err := newProvider(t.Context(), configFile{MCPServers: map[string]serverConfig{
		"demo": {Type: "http", URL: remote.URL},
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".mcp.json"),
		[]byte(`{"mcpServers":{"demo":{"type":"http","url":"http://127.0.0.1:1","enabled":false}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := provider.Snapshot(tools.WithAccess(t.Context(), tools.Access{Mode: permissions.FullAccess}), workspace)
	if err != nil || len(snapshot.Definitions) != 0 {
		t.Fatalf("disabled project override = %#v, %v", snapshot, err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".mcp.json"),
		[]byte(`{"mcpServers":{"demo":{"type":"http","url":"http://127.0.0.1:1"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err = provider.Snapshot(tools.WithAccess(t.Context(), tools.Access{Mode: permissions.FullAccess}), workspace)
	if err != nil || len(snapshot.Definitions) != 0 {
		t.Fatalf("failed project override fell back: %#v, %v", snapshot, err)
	}
}

func TestProvider_newRunUsesNewConnectionAndOldRunKeepsOld(t *testing.T) {
	makeRemote := func(label string, count *atomic.Int32) *httptest.Server {
		server := sdk.NewServer(&sdk.Implementation{Name: label, Version: "v1"}, nil)
		server.AddTool(&sdk.Tool{Name: "ping", InputSchema: json.RawMessage(`{"type":"object"}`)},
			func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				count.Add(1)
				return &sdk.CallToolResult{}, nil
			})
		return httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server },
			&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	}
	var oldCalls, newCalls atomic.Int32
	oldRemote := makeRemote("old", &oldCalls)
	defer oldRemote.Close()
	newRemote := makeRemote("new", &newCalls)
	defer newRemote.Close()
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider, err := New(files, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	view, err := provider.ReadSettings("")
	if err != nil {
		t.Fatal(err)
	}
	protocol := "http"
	oldURL := oldRemote.URL
	view, err = provider.Save(t.Context(), SaveInput{Name: "demo", Revision: view.Revision,
		Create: true, Type: &protocol, URL: &oldURL})
	if err != nil {
		t.Fatal(err)
	}
	oldAccess := tools.Access{Mode: permissions.FullAccess, SessionID: "s", RunID: "old"}
	_, err = provider.Snapshot(tools.WithAccess(t.Context(), oldAccess), "")
	if err != nil {
		t.Fatal(err)
	}
	oldState := provider.runs[runKey("s", "old")]
	newURL := newRemote.URL
	allTools := []string{}
	_, err = provider.Save(t.Context(), SaveInput{Name: "demo", Revision: view.Revision,
		Create: false, URL: &newURL, IncludeTools: &allTools})
	if err != nil {
		t.Fatal(err)
	}
	newAccess := tools.Access{Mode: permissions.FullAccess, SessionID: "s", RunID: "new"}
	_, err = provider.Snapshot(tools.WithAccess(t.Context(), newAccess), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"old", "new"} {
		_, err = provider.Call(t.Context(), tools.Call{Name: "mcp__demo__ping", Arguments: json.RawMessage(`{}`),
			Mode: permissions.FullAccess, SessionID: "s", RunID: id})
		if err != nil {
			t.Fatal(err)
		}
	}
	if oldCalls.Load() != 1 || newCalls.Load() != 1 {
		t.Fatalf("runs used wrong connections: old=%d new=%d", oldCalls.Load(), newCalls.Load())
	}
	provider.ReleaseRun("s", "old")
	if !oldState.closed {
		t.Fatal("retired connection was not released after old run")
	}
	provider.ReleaseRun("s", "new")
}

func TestMCPStdioHelper(t *testing.T) {
	if os.Getenv("HARNESS_MCP_STDIO_HELPER") != "1" {
		return
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "stdio-helper", Version: "v1"}, nil)
	server.AddTool(&sdk.Tool{Name: "ping", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		})
	if err := server.Run(t.Context(), &sdk.StdioTransport{}); err != nil {
		t.Fatal(err)
	}
}

func TestProvider_stdioConnection(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := newProvider(t.Context(), configFile{MCPServers: map[string]serverConfig{
		"local": {Type: "stdio", Command: executable, Args: []string{"-test.run=^TestMCPStdioHelper$"},
			Env: map[string]string{"HARNESS_MCP_STDIO_HELPER": "1"}},
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	snapshot, err := provider.Snapshot(tools.WithAccess(t.Context(), tools.Access{Mode: permissions.FullAccess}), "")
	if err != nil || len(snapshot.Definitions) != 1 || snapshot.Definitions[0].Name != "mcp__local__ping" {
		t.Fatalf("stdio discovery = %#v, %v", snapshot, err)
	}
}

func TestProvider_oneFailedServerDoesNotHideOthers(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "healthy", Version: "v1"}, nil)
	server.AddTool(&sdk.Tool{Name: "ping", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		})
	remote := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer remote.Close()
	provider, err := newProvider(t.Context(), configFile{MCPServers: map[string]serverConfig{
		"broken":  {Type: "http", URL: "http://127.0.0.1:1"},
		"healthy": {Type: "http", URL: remote.URL},
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	snapshot, err := provider.Snapshot(tools.WithAccess(t.Context(), tools.Access{Mode: permissions.FullAccess}), "")
	if err != nil || len(snapshot.Definitions) != 1 || snapshot.Definitions[0].Name != "mcp__healthy__ping" {
		t.Fatalf("failed server hid healthy tool: %#v, %v", snapshot, err)
	}
	view, err := provider.ReadSettings("")
	if err != nil || len(view.Global) != 2 || view.Global[0].Status != "failed" || view.Global[1].Status != "connected" {
		t.Fatalf("connection statuses = %#v, %v", view.Global, err)
	}
}
