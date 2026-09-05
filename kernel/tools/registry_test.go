package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// 数据。registry 测试工具的参数。
type testArgs struct {
	Path string `json:"path" jsonschema:"minLength=1,description=Path to use."`
}

func TestRegistry_generatesSchemaValidatesAndCalls(t *testing.T) {
	registry := NewRegistry()
	var got testArgs
	tool := New("test", "Test tool.", func(_ context.Context, _ Call, args testArgs) (Result, error) {
		got = args
		return Result{Content: "ok"}, nil
	})
	err := registry.Register(tool)
	if err != nil {
		t.Fatal(err)
	}

	definitions, err := registry.Definitions(context.Background(), "", []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	err = json.Unmarshal(definitions[0].InputSchema, &schema)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	path := properties["path"].(map[string]any)
	if path["minLength"] != float64(1) {
		t.Fatalf("schema minLength = %v, want 1", path["minLength"])
	}

	result, err := registry.Call(context.Background(), Call{
		Name:      "test",
		Arguments: json.RawMessage(`{"path":"a.txt"}`),
		Allow:     []string{"test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || result.Content != "ok" {
		t.Fatalf("result = %#v", result)
	}
	if got.Path != "a.txt" {
		t.Fatalf("handler args = %#v", got)
	}
}

func TestRegistry_rejectsInvalidOrUnauthorizedCall(t *testing.T) {
	registry := NewRegistry()
	called := false
	tool := New("test", "Test tool.", func(_ context.Context, _ Call, _ testArgs) (Result, error) {
		called = true
		return Result{}, nil
	})
	err := registry.Register(tool)
	if err != nil {
		t.Fatal(err)
	}

	invalid, err := registry.Call(context.Background(), Call{
		Name:      "test",
		Arguments: json.RawMessage(`{"path":""}`),
		Allow:     []string{"test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !invalid.IsError || called {
		t.Fatalf("invalid result = %#v, called = %t", invalid, called)
	}

	unauthorized, err := registry.Call(context.Background(), Call{
		Name:      "test",
		Arguments: json.RawMessage(`{"path":"a.txt"}`),
		Allow:     nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !unauthorized.IsError || !strings.Contains(unauthorized.Content, "not allowed") {
		t.Fatalf("unauthorized result = %#v", unauthorized)
	}
}

func TestRegistry_convertsToolErrorAndPreservesCancellation(t *testing.T) {
	registry := NewRegistry()
	tool := New("test", "Test tool.", func(_ context.Context, _ Call, _ testArgs) (Result, error) {
		return Result{}, errors.New("failed")
	})
	err := registry.Register(tool)
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Call(context.Background(), Call{
		Name:      "test",
		Arguments: json.RawMessage(`{"path":"a.txt"}`),
		Allow:     []string{"test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content != "failed" {
		t.Fatalf("result = %#v", result)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = registry.Call(ctx, Call{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestRegistry_listReturnsAllDefinitionsByName(t *testing.T) {
	registry := NewRegistry()
	for _, name := range []string{"write", "bash"} {
		tool := New(name, name+" tool.", func(_ context.Context, _ Call, _ testArgs) (Result, error) {
			return Result{}, nil
		})
		err := registry.Register(tool)
		if err != nil {
			t.Fatal(err)
		}
	}
	definitions := registry.List()
	if len(definitions) != 2 || definitions[0].Name != "bash" || definitions[1].Name != "write" {
		t.Fatalf("List() = %#v", definitions)
	}
}

func TestRegistry_prepareAddsProviderToolsWithoutListingThem(t *testing.T) {
	registry := NewRegistry()
	tool := New("bash", "Bash tool.", func(_ context.Context, _ Call, _ testArgs) (Result, error) {
		return Result{}, nil
	})
	err := registry.Register(tool)
	if err != nil {
		t.Fatal(err)
	}
	err = registry.RegisterProvider(testToolProvider{
		name: "mcp",
		snapshot: Snapshot{
			Definitions: []Definition{{
				Name:        "mcp__demo__greet",
				Description: "Greet.",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := registry.List()
	if len(list) != 1 || list[0].Name != "bash" {
		t.Fatalf("List() = %#v", list)
	}
	prepared, err := registry.Prepare(context.Background(), "/work", []string{"bash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Names) != 2 || prepared.Names[0] != "bash" || prepared.Names[1] != "mcp__demo__greet" {
		t.Fatalf("Prepare() = %#v", prepared)
	}
}

type testToolProvider struct {
	name     string
	snapshot Snapshot
}

func (p testToolProvider) Name() string { return p.name }

func (p testToolProvider) Snapshot(context.Context, string) (Snapshot, error) {
	return p.snapshot, nil
}

func (p testToolProvider) Call(context.Context, Call) (Result, error) {
	return Result{Content: "ok"}, nil
}

func TestTruncate(t *testing.T) {
	head := TruncateHead(strings.Repeat("a", maxOutputBytes+1))
	if !strings.HasSuffix(head, truncatedNotice) {
		t.Fatalf("head = %q", head[len(head)-100:])
	}
	tail := TruncateTail(strings.Repeat("a", maxOutputBytes+1))
	if !strings.HasPrefix(tail, truncatedNotice) {
		t.Fatalf("tail = %q", tail[:100])
	}
}
