package react

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/session"
	"harness/kernel/tools"
)

// 数据。测试工具的模型参数。
type echoArgs struct {
	Value string `json:"value" jsonschema:"minLength=1"`
}

func TestReactRunsToolRoundTrip(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		requests = append(requests, body)
		step := len(requests)
		mu.Unlock()
		if step == 1 {
			writeSSE(w,
				`{"choices":[{"delta":{"reasoning_content":"think"},"index":0}]}`,
				`{"choices":[{"delta":{"content":"checking"},"index":0}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"value\":\"hello\"}"}}]},"index":0}]}`,
				`{"choices":[{"delta":{},"index":0,"finish_reason":"tool_calls"}]}`,
			)
			return
		}
		writeSSE(w,
			`{"choices":[{"delta":{"content":"done"},"index":0}]}`,
			`{"choices":[{"delta":{},"index":0,"finish_reason":"stop"}]}`,
		)
	}))
	defer server.Close()

	loop, toolRegistry := installReact(t, server.URL)
	var gotWorkspace string
	_, err := toolRegistry.Register(tools.New("echo", "Echo a value.", func(_ context.Context, call tools.Call, args echoArgs) (tools.Result, error) {
		gotWorkspace = call.Workspace
		return tools.Result{Content: args.Value}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	var events []loops.Event
	checkpoints := 0
	err = loop.Run(t.Context(), loops.Invocation{
		History: []session.Message{{
			Role:   session.RoleUser,
			Blocks: []session.Block{{Kind: "text", Text: "use echo"}},
		}},
		SystemPrompt: "system",
		LLMConfig: llm.RunConfig{
			Model:           "deepseek/deepseek-v4-flash",
			ReasoningEffort: "off",
		},
		ToolNames: []string{"echo"},
		Workspace: "/workspace",
		Emit: func(_ context.Context, event loops.Event) error {
			events = append(events, event)
			return nil
		},
		Checkpoint: func(context.Context) ([]session.Message, error) {
			checkpoints++
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotWorkspace != "/workspace" {
		t.Fatalf("workspace = %q", gotWorkspace)
	}
	if checkpoints != 2 {
		t.Fatalf("checkpoints = %d, want 2", checkpoints)
	}

	wantKinds := []loops.EventKind{
		loops.EventReasoningDelta,
		loops.EventTextDelta,
		loops.EventMessage,
		loops.EventToolStarted,
		loops.EventMessage,
		loops.EventToolFinished,
		loops.EventTextDelta,
		loops.EventMessage,
	}
	gotKinds := make([]loops.EventKind, 0, len(events))
	for _, event := range events {
		gotKinds = append(gotKinds, event.Kind)
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("event kinds = %#v, want %#v", gotKinds, wantKinds)
	}
	result := events[4].Message.Blocks[0].Result
	if result.ID != "call_1" || result.Name != "echo" || result.Content != "hello" || result.IsError {
		t.Fatalf("tool result = %#v", result)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	for index, request := range requests {
		if tools, ok := request["tools"].([]any); !ok || len(tools) != 1 {
			t.Fatalf("request %d tools = %#v", index, request["tools"])
		}
	}
	messages := requests[1]["messages"].([]any)
	toolMessage := messages[len(messages)-1].(map[string]any)
	if toolMessage["role"] != "tool" || toolMessage["tool_call_id"] != "call_1" || toolMessage["content"] != "hello" {
		t.Fatalf("tool message = %#v", toolMessage)
	}
}

func TestReactExecutesToolCallsInModelOrder(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			writeSSE(w,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"value\":\"one\"}"}},{"index":1,"id":"call_2","type":"function","function":{"name":"echo","arguments":"{\"value\":\"two\"}"}}]},"index":0}]}`,
				`{"choices":[{"delta":{},"index":0,"finish_reason":"tool_calls"}]}`,
			)
			return
		}
		writeSSE(w, `{"choices":[{"delta":{"content":"done"},"index":0,"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	loop, toolRegistry := installReact(t, server.URL)
	var order []string
	_, err := toolRegistry.Register(tools.New("echo", "Echo a value.", func(_ context.Context, _ tools.Call, args echoArgs) (tools.Result, error) {
		order = append(order, args.Value)
		return tools.Result{Content: args.Value}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = loop.Run(t.Context(), testInvocation([]string{"echo"}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"one", "two"}) {
		t.Fatalf("tool order = %#v", order)
	}
}

func TestReactReturnsUnauthorizedToolFailureToModel(t *testing.T) {
	var secondRequest map[string]any
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			writeSSE(w,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"missing","arguments":"{}"}}]},"index":0}]}`,
				`{"choices":[{"delta":{},"index":0,"finish_reason":"tool_calls"}]}`,
			)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&secondRequest); err != nil {
			t.Error(err)
			return
		}
		writeSSE(w, `{"choices":[{"delta":{"content":"recovered"},"index":0,"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	loop, _ := installReact(t, server.URL)
	var result *session.ToolResult
	invocation := testInvocation(nil)
	invocation.Emit = func(_ context.Context, event loops.Event) error {
		if event.Message != nil && event.Message.Role == session.RoleTool {
			result = event.Message.Blocks[0].Result
		}
		return nil
	}
	if err := loop.Run(t.Context(), invocation); err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError || result.Content == "" {
		t.Fatalf("tool result = %#v", result)
	}
	messages := secondRequest["messages"].([]any)
	toolMessage := messages[len(messages)-1].(map[string]any)
	if toolMessage["content"] != result.Content {
		t.Fatalf("model tool result = %#v, ledger result = %#v", toolMessage["content"], result.Content)
	}
}

func TestReactReturnsMalformedToolArgumentsToModel(t *testing.T) {
	var secondRequest map[string]any
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			writeSSE(w,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"not-json"}}]},"index":0}]}`,
				`{"choices":[{"delta":{},"index":0,"finish_reason":"tool_calls"}]}`,
			)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&secondRequest); err != nil {
			t.Error(err)
			return
		}
		writeSSE(w, `{"choices":[{"delta":{"content":"recovered"},"index":0,"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	loop, toolRegistry := installReact(t, server.URL)
	_, err := toolRegistry.Register(tools.New("echo", "Echo a value.", func(_ context.Context, _ tools.Call, _ echoArgs) (tools.Result, error) {
		return tools.Result{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Run(t.Context(), testInvocation([]string{"echo"})); err != nil {
		t.Fatal(err)
	}
	messages := secondRequest["messages"].([]any)
	toolMessage := messages[len(messages)-1].(map[string]any)
	if toolMessage["role"] != "tool" || toolMessage["tool_call_id"] != "call_1" {
		t.Fatalf("tool message = %#v", toolMessage)
	}
	if !strings.Contains(toolMessage["content"].(string), "arguments are not valid JSON") {
		t.Fatalf("tool message = %#v", toolMessage)
	}
}

func TestReactAddsCheckpointMessagesBeforeNextRequest(t *testing.T) {
	var secondRequest map[string]any
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			writeSSE(w, `{"choices":[{"delta":{"content":"first"},"index":0,"finish_reason":"stop"}]}`)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&secondRequest); err != nil {
			t.Error(err)
			return
		}
		writeSSE(w, `{"choices":[{"delta":{"content":"second"},"index":0,"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	loop, _ := installReact(t, server.URL)
	checkpoint := 0
	invocation := testInvocation(nil)
	invocation.Checkpoint = func(context.Context) ([]session.Message, error) {
		checkpoint++
		if checkpoint == 1 {
			return []session.Message{{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "steer"}}}}, nil
		}
		return nil, nil
	}
	if err := loop.Run(t.Context(), invocation); err != nil {
		t.Fatal(err)
	}
	messages := secondRequest["messages"].([]any)
	last := messages[len(messages)-1].(map[string]any)
	if last["role"] != "user" || last["content"] != "steer" {
		t.Fatalf("last message = %#v", last)
	}
}

func TestReactStopsOnEmitFailureAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSSE(w, `{"choices":[{"delta":{"content":"text"},"index":0,"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	loop, _ := installReact(t, server.URL)
	want := errors.New("emit failed")
	invocation := testInvocation(nil)
	invocation.Emit = func(context.Context, loops.Event) error {
		return want
	}
	if err := loop.Run(t.Context(), invocation); !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want emit failure", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := loop.Run(ctx, testInvocation(nil)); err == nil {
		t.Fatal("Run() error = nil, want cancellation")
	}
}

func installReact(t *testing.T, baseURL string) (loops.Loop, tools.Tools) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(home+"/.harness", 0o755); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("providers:\n  deepseek:\n    apiKey: test-key\n    baseURL: %s\n", baseURL)
	if err := os.WriteFile(home+"/.harness/config.yaml", []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	h := host.NewHost()
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, plugin := range []host.Plugin{&llm.Plugin{}, tools.NewPlugin(), loops.NewPlugin(), New()} {
		if err := h.Install(plugin); err != nil {
			t.Fatal(err)
		}
	}
	loopRegistry, err := host.Resolve[loops.Loops](h, "loops")
	if err != nil {
		t.Fatal(err)
	}
	loop, err := loopRegistry.Get("react")
	if err != nil {
		t.Fatal(err)
	}
	toolRegistry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		t.Fatal(err)
	}
	return loop, toolRegistry
}

func testInvocation(toolNames []string) loops.Invocation {
	return loops.Invocation{
		History: []session.Message{{
			Role:   session.RoleUser,
			Blocks: []session.Block{{Kind: "text", Text: "test"}},
		}},
		SystemPrompt: "system",
		LLMConfig: llm.RunConfig{
			Model:           "deepseek/deepseek-v4-flash",
			ReasoningEffort: "off",
		},
		ToolNames: toolNames,
		Workspace: "/workspace",
		Emit: func(context.Context, loops.Event) error {
			return nil
		},
		Checkpoint: func(context.Context) ([]session.Message, error) {
			return nil, nil
		},
	}
}

func writeSSE(w http.ResponseWriter, frames ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, frame := range frames {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", frame)
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}
