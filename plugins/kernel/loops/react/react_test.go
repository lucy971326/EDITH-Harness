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
	"sync/atomic"
	"testing"
	"time"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
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
	err := toolRegistry.Register(tools.New("echo", "Echo a value.", func(_ context.Context, call tools.Call, args echoArgs) (tools.Result, error) {
		gotWorkspace = call.Workspace
		return tools.Result{Content: args.Value}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	var events []loops.Event
	var checkpoints []loops.CheckpointPhase
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
		Checkpoint: func(_ context.Context, phase loops.CheckpointPhase) ([]session.Message, error) {
			checkpoints = append(checkpoints, phase)
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotWorkspace != "/workspace" {
		t.Fatalf("workspace = %q", gotWorkspace)
	}
	wantCheckpoints := []loops.CheckpointPhase{loops.CheckpointContinue, loops.CheckpointFinal}
	if !reflect.DeepEqual(checkpoints, wantCheckpoints) {
		t.Fatalf("checkpoints = %v, want %v", checkpoints, wantCheckpoints)
	}

	wantKinds := []loops.EventKind{
		loops.EventReasoningDelta,
		loops.EventTextDelta,
		loops.EventUsage,
		loops.EventMessage,
		loops.EventToolStarted,
		loops.EventMessage,
		loops.EventToolFinished,
		loops.EventTextDelta,
		loops.EventUsage,
		loops.EventMessage,
	}
	gotKinds := make([]loops.EventKind, 0, len(events))
	for _, event := range events {
		gotKinds = append(gotKinds, event.Kind)
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("event kinds = %#v, want %#v", gotKinds, wantKinds)
	}
	var result *session.ToolResult
	for _, event := range events {
		if event.Message != nil && event.Message.Role == session.RoleTool && len(event.Message.Blocks) > 0 {
			result = event.Message.Blocks[0].Result
			break
		}
	}
	if result == nil || result.ID != "call_1" || result.Name != "echo" || result.Content != "hello" || result.IsError {
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

func TestReactEmitsUsageAfterEachStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSSE(w,
			`{"choices":[{"delta":{"content":"ok"},"index":0}]}`,
			`{"choices":[{"delta":{},"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":31000,"completion_tokens":20,"total_tokens":31020,"prompt_tokens_details":{"cached_tokens":19000}}}`,
		)
	}))
	defer server.Close()

	loop, _ := installReact(t, server.URL)
	var usages []*loops.Usage
	invocation := testInvocation(nil)
	invocation.Emit = func(_ context.Context, event loops.Event) error {
		if event.Kind == loops.EventUsage {
			usages = append(usages, event.Usage)
		}
		return nil
	}
	if err := loop.Run(t.Context(), invocation); err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0] == nil {
		t.Fatalf("usages = %#v", usages)
	}
	got := usages[0]
	if got.InputTokens != 12000 || got.CacheReadTokens != 19000 || got.ContextWindow != 1000000 {
		t.Fatalf("usage = %#v", got)
	}
}

func TestReactEmitsZeroUsageWhenProviderOmitsIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeSSE(w, `{"choices":[{"delta":{"content":"ok"},"index":0,"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	loop, _ := installReact(t, server.URL)
	var got *loops.Usage
	invocation := testInvocation(nil)
	invocation.Emit = func(_ context.Context, event loops.Event) error {
		if event.Kind == loops.EventUsage {
			got = event.Usage
		}
		return nil
	}
	if err := loop.Run(t.Context(), invocation); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.InputTokens != 0 || got.CacheReadTokens != 0 || got.ContextWindow != 1000000 {
		t.Fatalf("usage = %#v", got)
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
	err := toolRegistry.Register(tools.New("echo", "Echo a value.", func(_ context.Context, _ tools.Call, args echoArgs) (tools.Result, error) {
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
	err := toolRegistry.Register(tools.New("echo", "Echo a value.", func(_ context.Context, _ tools.Call, _ echoArgs) (tools.Result, error) {
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

func TestReactPersistsCancelledToolResult(t *testing.T) {
	registry := tools.NewRegistry()
	if err := registry.Register(tools.New("echo", "Echo a value.", func(context.Context, tools.Call, echoArgs) (tools.Result, error) {
		return tools.Result{Content: "unexpected"}, nil
	})); err != nil {
		t.Fatal(err)
	}
	loop := &reactLoop{tools: registry}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var events []loops.Event
	message, err := loop.execute(ctx, loops.Invocation{
		ToolNames: []string{"echo"},
		Emit: func(_ context.Context, event loops.Event) error {
			events = append(events, event)
			return nil
		},
	}, session.ToolCall{ID: "call_1", Name: "echo", Args: `{"value":"hello"}`}, 1, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("execute error = %v", err)
	}
	result := message.Blocks[0].Result
	if result == nil || !result.IsError || result.Content != "已取消" {
		t.Fatalf("tool result = %#v", result)
	}
	if len(events) != 3 || events[0].Kind != loops.EventToolStarted || events[1].Kind != loops.EventMessage || events[2].Kind != loops.EventToolFinished {
		t.Fatalf("events = %#v", events)
	}
	if !events[2].Tool.IsError {
		t.Fatalf("finish event = %#v", events[2])
	}
}

func TestReactCancellingOneOfMultipleToolCallsPersistsEveryResult(t *testing.T) {
	var requests atomic.Int32
	toolStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeSSE(w,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"value\":\"one\"}"}},{"index":1,"id":"call_2","type":"function","function":{"name":"echo","arguments":"{\"value\":\"two\"}"}}]},"index":0}]}`,
			`{"choices":[{"delta":{},"index":0,"finish_reason":"tool_calls"}]}`,
		)
	}))
	defer server.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	err := os.MkdirAll(home+"/.harness", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("providers:\n  deepseek:\n    apiKey: test-key\n    baseURL: %s\n", server.URL)
	err = os.WriteFile(home+"/.harness/config.yaml", []byte(config), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	h := host.NewHost()
	t.Cleanup(func() {
		err := h.Close()
		if err != nil {
			t.Error(err)
		}
	})
	for _, plugin := range []host.Plugin{
		&persist.Plugin{Dir: t.TempDir()},
		&session.Plugin{},
		&llm.Plugin{},
		tools.NewPlugin(),
		events.NewPlugin(),
		loops.NewPlugin(),
		New(),
	} {
		err = h.Install(plugin)
		if err != nil {
			t.Fatal(err)
		}
	}

	toolRegistry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		t.Fatal(err)
	}
	var called []string
	var calledMu sync.Mutex
	err = toolRegistry.Register(tools.New("echo", "Echo a value.", func(ctx context.Context, _ tools.Call, args echoArgs) (tools.Result, error) {
		calledMu.Lock()
		called = append(called, args.Value)
		calledMu.Unlock()
		if args.Value == "one" {
			close(toolStarted)
			<-ctx.Done()
			return tools.Result{}, ctx.Err()
		}
		return tools.Result{Content: args.Value}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, plugin := range []host.Plugin{
		skills.NewPlugin(),
		agents.NewPlugin(),
		runner.NewPlugin(),
	} {
		err = h.Install(plugin)
		if err != nil {
			t.Fatal(err)
		}
	}

	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := sessions.Create("session-1")
	if err != nil {
		t.Fatal(err)
	}
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		t.Fatal(err)
	}
	err = settingsStore.Put("session-1", settings.SessionSettings{
		AgentID:         agents.DefaultID,
		Model:           "deepseek/deepseek-v4-flash",
		ReasoningEffort: "off",
		Workspace:       "/workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := host.Resolve[*runner.Runner](h, "runner")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- r.Run(context.Background(), "session-1", session.UserMessage{
			Blocks: []session.Block{{Kind: "text", Text: "run both"}},
		})
	}()
	select {
	case <-toolStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first tool did not start")
	}
	err = r.Stop("session-1")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled run did not finish")
	}

	disk, err := host.Resolve[persist.Persistence](h, "sessionPersistence")
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := session.NewStore(disk).Get("session-1")
	if err != nil {
		t.Fatal(err)
	}
	history := reloaded.History()
	if !reflect.DeepEqual(history, sess.History()) {
		t.Fatal("persisted history differs from live history")
	}
	if len(history) != 4 {
		t.Fatalf("history length = %d, want user, assistant, and two tool results: %#v", len(history), history)
	}
	assistant := history[1]
	if assistant.Role != session.RoleAssistant || len(assistant.Blocks) != 2 {
		t.Fatalf("assistant message = %#v", assistant)
	}
	wantResults := []struct {
		id      string
		content string
	}{
		{id: "call_1", content: "已取消"},
		{id: "call_2", content: "未执行：任务已停止"},
	}
	for i, want := range wantResults {
		message := history[i+2]
		if message.Role != session.RoleTool || len(message.Blocks) != 1 || message.Blocks[0].Result == nil {
			t.Fatalf("history[%d] = %#v, want tool result", i+2, message)
		}
		result := message.Blocks[0].Result
		if result.ID != want.id || result.Name != "echo" || result.Content != want.content || !result.IsError {
			t.Fatalf("history[%d] result = %#v, want id=%q content=%q error", i+2, result, want.id, want.content)
		}
	}
	calledMu.Lock()
	gotCalled := append([]string(nil), called...)
	calledMu.Unlock()
	if !reflect.DeepEqual(gotCalled, []string{"one"}) {
		t.Fatalf("executed tools = %#v, want only first tool", gotCalled)
	}
	if requests.Load() != 1 {
		t.Fatalf("model requests = %d, want 1", requests.Load())
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
	invocation.Checkpoint = func(context.Context, loops.CheckpointPhase) ([]session.Message, error) {
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
		Checkpoint: func(context.Context, loops.CheckpointPhase) ([]session.Message, error) {
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
