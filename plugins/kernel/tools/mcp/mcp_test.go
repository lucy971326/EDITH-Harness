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

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"harness/kernel/tools"
)

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
	p, err := newProvider(t.Context(), filepath.Join(t.TempDir(), "missing.json"), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	_, err = p.Snapshot(t.Context(), workspace)
	if err == nil {
		t.Fatal("first discovery should fail")
	}
	available.Store(true)
	snapshot, err := p.Snapshot(t.Context(), workspace)
	if err != nil {
		t.Fatalf("discovery after server recovery: %v", err)
	}
	if len(snapshot.Definitions) != 1 || snapshot.Definitions[0].Name != "mcp__demo__ping" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	count := requests.Load()
	available.Store(false)
	_, err = p.Snapshot(t.Context(), workspace)
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
	provider, err := newProvider(context.Background(), filepath.Join(t.TempDir(), "missing.json"), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	registry := tools.NewRegistry()
	err = registry.RegisterProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := registry.Prepare(context.Background(), workspace, nil)
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
		Allow:     prepared.Names,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, "hello Lucy") || !strings.Contains(result.Content, `"name":"Lucy"`) {
		t.Fatalf("result = %#v", result)
	}
}
