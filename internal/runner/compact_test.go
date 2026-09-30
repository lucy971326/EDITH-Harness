package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"harness/internal/agents"
	"harness/internal/events"
	"harness/internal/llm"
	"harness/internal/loops"
	"harness/internal/persist"
	"harness/internal/session"
	"harness/internal/session/settings"
	"harness/internal/tools"
)

type pingArgs struct{}

func TestCompactRejectsEmptyHistory(t *testing.T) {
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error { return nil }})
	err := fixture.runner.Compact(context.Background(), "session-1")
	if err == nil || !strings.Contains(err.Error(), "no history") {
		t.Fatalf("Compact() error = %v", err)
	}
}

func TestCompactRejectsRunningSession(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, _ loops.Invocation) error {
		close(started)
		<-block
		return ctx.Err()
	}})
	t.Cleanup(func() { close(block) })
	_, err := fixture.runner.Start(context.Background(), "session-1", textInput("hi"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not start")
	}
	err = fixture.runner.Compact(context.Background(), "session-1")
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("Compact() error = %v", err)
	}
}

func TestCompactAppendsSummaryAndProjectsHistory(t *testing.T) {
	var request map[string]any
	fixture := newCompactFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		writeCompactSSE(w,
			`{"choices":[{"delta":{"content":"keep going"},"index":0}]}`,
			`{"choices":[{"delta":{},"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}`,
		)
	})
	seedCompactHistory(t, fixture.session)
	ended := waitRunEnded(t, fixture.events)
	err := fixture.runner.Compact(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	event := <-ended
	if event.Status != RunSucceeded {
		t.Fatalf("status = %s error = %s", event.Status, event.Error)
	}
	entries := fixture.session.Entries()
	if len(entries) != 5 || entries[4].Message.Blocks[0].Kind != "summary" || entries[4].Message.Blocks[0].Text != "keep going" {
		t.Fatalf("entries = %#v", entries)
	}
	history := fixture.session.History()
	if len(history) != 3 || history[0].Role != session.RoleUser || history[0].Blocks[0].Kind != "text" || history[0].Blocks[0].Text != session.SummaryText("keep going") || history[1].Blocks[0].Text != "latest request" || history[2].Blocks[0].Text != "recent answer" {
		t.Fatalf("history = %#v", history)
	}
	if _, ok := request["tools"]; ok {
		t.Fatalf("compact request included tools: %#v", request["tools"])
	}
	if _, ok := request["tool_choice"]; ok {
		t.Fatalf("compact request included tool choice: %#v", request["tool_choice"])
	}
	messages, _ := request["messages"].([]any)
	if len(messages) == 0 {
		t.Fatal("missing messages")
	}
	last, _ := messages[len(messages)-1].(map[string]any)
	if last["role"] != "user" || !strings.Contains(fmt.Sprint(last["content"]), "摘要") {
		t.Fatalf("last message = %#v", last)
	}
}

func TestCompactDoesNotCommitOnLengthOrEmpty(t *testing.T) {
	t.Run("length", func(t *testing.T) {
		assertCompactDoesNotCommit(t, "truncated",
			`{"choices":[{"delta":{"content":"partial"},"index":0}]}`,
			`{"choices":[{"delta":{},"index":0,"finish_reason":"length"}]}`,
		)
	})
	t.Run("empty", func(t *testing.T) {
		assertCompactDoesNotCommit(t, "empty summary",
			`{"choices":[{"delta":{},"index":0,"finish_reason":"stop"}]}`,
		)
	})
}

func TestCompactWriteFailureKeepsHistory(t *testing.T) {
	fixture := newCompactFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCompactSSE(w, `{"choices":[{"delta":{"content":"summary"},"index":0}]}`, `{"choices":[{"delta":{},"index":0,"finish_reason":"stop"}]}`)
	})
	seedCompactHistory(t, fixture.session)
	fixture.persistence.mu.Lock()
	fixture.persistence.addFail = errors.New("disk full")
	fixture.persistence.mu.Unlock()
	ended := waitRunEnded(t, fixture.events)
	if err := fixture.runner.Compact(t.Context(), "session-1"); err != nil {
		t.Fatal(err)
	}
	result := <-ended
	if result.Status != RunFailed || !strings.Contains(result.Error, "disk full") || len(fixture.session.History()) != 4 {
		t.Fatalf("result=%+v", result)
	}
}

func TestAutomaticCompactReplacesContextWithinRun(t *testing.T) {
	var requests atomic.Int32
	fixture := newCompactFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeCompactSSE(w, `{"choices":[{"delta":{"content":"summary"},"index":0}]}`, `{"choices":[{"delta":{},"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":20000,"completion_tokens":2}}`)
	})
	seedCompactHistory(t, fixture.session)
	fixture.runner.loop = &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		current, err := fixture.runner.current("session-1")
		if err != nil {
			return err
		}
		current.mu.Lock()
		current.usage = &Usage{InputTokens: 2000000}
		current.inputEstimate = 1
		current.mu.Unlock()
		input := llm.Input{System: invocation.SystemPrompt, History: invocation.History}
		history, err := invocation.Compact(ctx, fixture.runner.llm.Pin(), input, false)
		if err != nil {
			return err
		}
		if len(history) == 0 || !strings.Contains(fmt.Sprint(history), "历史交接摘要") {
			return fmt.Errorf("missing replacement")
		}
		input.History = history
		again, err := invocation.Compact(ctx, fixture.runner.llm.Pin(), input, false)
		if again != nil || err != nil {
			return fmt.Errorf("repeated compaction: %v", err)
		}
		current.mu.Lock()
		usage := *current.usage
		current.mu.Unlock()
		if usage.EstimatedTokens <= 0 || usage.EstimatedTokens >= usage.InputTokens {
			return fmt.Errorf("invalid usage: %+v", usage)
		}
		return nil
	}}
	if err := fixture.runner.Run(t.Context(), "session-1", textInput("continue")); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d", requests.Load())
	}
}

func TestCompactSelectionKeepsToolBatch(t *testing.T) {
	entries := []session.Entry{
		{ID: "user", Message: session.Message{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "goal"}}}},
		{ID: "old", Message: session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: strings.Repeat("old ", 1000)}}}},
		{ID: "call", Message: session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "tool-call", Tool: &session.ToolCall{ID: "a", Name: "read", Args: "{}"}}, {Kind: "tool-call", Tool: &session.ToolCall{ID: "b", Name: "read", Args: "{}"}}}}},
		{ID: "a", Message: session.Message{Role: session.RoleTool, Blocks: []session.Block{{Kind: "tool-result", Result: &session.ToolResult{ID: "a", Name: "read", Content: "a"}}}}},
		{ID: "b", Message: session.Message{Role: session.RoleTool, Blocks: []session.Block{{Kind: "tool-result", Result: &session.ToolResult{ID: "b", Name: "read", Content: "b"}}}}},
	}
	history := []session.Message{}
	for _, e := range entries {
		history = append(history, e.Message)
	}
	cut, retained, err := compactSelection(entries, history, 128)
	if err != nil || cut != 2 || strings.Join(retained, ",") != "user,call,a,b" {
		t.Fatalf("cut=%d keep=%v err=%v", cut, retained, err)
	}
	_, _, err = compactSelection(entries[:4], history[:4], 128)
	if err == nil {
		t.Fatal("accepted incomplete batch")
	}
}

func TestCompactStopDoesNotCommit(t *testing.T) {
	started := make(chan struct{})
	fixture := newCompactFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(2 * time.Second)
		writeCompactSSE(w,
			`{"choices":[{"delta":{"content":"late"},"index":0}]}`,
			`{"choices":[{"delta":{},"index":0,"finish_reason":"stop"}]}`,
		)
	})
	seedCompactHistory(t, fixture.session)
	ended := waitRunEnded(t, fixture.events)
	err := fixture.runner.Compact(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("compact request did not start")
	}
	err = fixture.runner.Steer("session-1", textInput("must not enter compact"))
	if err == nil || !strings.Contains(err.Error(), "not running") {
		stopErr := fixture.runner.Stop("session-1")
		if stopErr != nil {
			t.Error(stopErr)
		}
		t.Fatalf("Steer during Compact error = %v", err)
	}
	history := fixture.session.History()
	if len(history) != 4 {
		t.Fatalf("history after rejected Compact Steer = %#v", history)
	}
	err = fixture.runner.Stop("session-1")
	if err != nil {
		t.Fatal(err)
	}
	event := <-ended
	if event.Status != RunCancelled {
		t.Fatalf("status = %s error = %s", event.Status, event.Error)
	}
	if len(fixture.session.Entries()) != 4 {
		t.Fatalf("entries = %#v", fixture.session.Entries())
	}
}

func assertCompactDoesNotCommit(t *testing.T, wantErr string, frames ...string) {
	t.Helper()
	fixture := newCompactFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCompactSSE(w, frames...)
	})
	seedCompactHistory(t, fixture.session)
	ended := waitRunEnded(t, fixture.events)
	err := fixture.runner.Compact(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	event := <-ended
	if event.Status != RunFailed || !strings.Contains(event.Error, wantErr) {
		t.Fatalf("ended = %#v want %q", event, wantErr)
	}
	if len(fixture.session.Entries()) != 4 {
		t.Fatalf("entries = %#v", fixture.session.Entries())
	}
}

func waitRunEnded(t *testing.T, registry *events.Registry) <-chan RunEvent {
	t.Helper()
	ended := make(chan RunEvent, 1)
	_, err := events.Subscribe(registry, func(_ context.Context, event RunEvent) error {
		if event.Kind == RunEnded {
			ended <- event
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ended
}

func newCompactFixture(t *testing.T, handler http.HandlerFunc) runnerFixture {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
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
	files, err := persist.NewFiles(home + "/.harness")
	if err != nil {
		t.Fatal(err)
	}
	client, err := llm.New(files)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newRunnerFixtureWithLLM(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error { return nil }}, client)
	err = fixture.tools.Register(tools.New("ping", "Ping.", func(context.Context, tools.Call, pingArgs) (tools.Result, error) {
		return tools.Result{Content: "pong"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	agent, err := fixture.agents.Get(agents.DefaultID)
	if err != nil {
		t.Fatal(err)
	}
	agent.Tools = []string{"ping"}
	_, err = fixture.agents.Save(agent)
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.settings.Put("session-1", settings.SessionSettings{
		AgentID:         agents.DefaultID,
		Model:           "deepseek/deepseek-flash",
		ReasoningEffort: "off",
		Workspace:       t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func writeCompactSSE(w http.ResponseWriter, frames ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, frame := range frames {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", frame)
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

func seedCompactHistory(t *testing.T, sess *session.Session) {
	t.Helper()
	for _, m := range []session.Message{
		{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "initial request"}}},
		{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: strings.Repeat("old findings ", 4000)}}},
		{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "latest request"}}},
		{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "recent answer"}}},
	} {
		if _, err := sess.Append(m); err != nil {
			t.Fatal(err)
		}
	}
}
