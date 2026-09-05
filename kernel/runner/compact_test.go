package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/tools"
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
	err := fixture.runner.Start(context.Background(), "session-1", textInput("hi"))
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
	_, err := fixture.session.Append(session.Message{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.session.Append(session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "a"}}})
	if err != nil {
		t.Fatal(err)
	}
	ended := waitRunEnded(t, fixture.events)
	err = fixture.runner.Compact(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	event := <-ended
	if event.Status != RunSucceeded {
		t.Fatalf("status = %s error = %s", event.Status, event.Error)
	}
	entries := fixture.session.Entries()
	if len(entries) != 3 || entries[2].Message.Blocks[0].Kind != "summary" || entries[2].Message.Blocks[0].Text != "keep going" {
		t.Fatalf("entries = %#v", entries)
	}
	history := fixture.session.History()
	if len(history) != 1 || history[0].Blocks[0].Kind != "text" || history[0].Blocks[0].Text != "keep going" {
		t.Fatalf("history = %#v", history)
	}
	if request["tool_choice"] != "none" {
		t.Fatalf("tool_choice = %#v", request["tool_choice"])
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
	_, err := fixture.session.Append(session.Message{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	ended := waitRunEnded(t, fixture.events)
	err = fixture.runner.Compact(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("compact request did not start")
	}
	err = fixture.runner.Stop("session-1")
	if err != nil {
		t.Fatal(err)
	}
	event := <-ended
	if event.Status != RunCancelled {
		t.Fatalf("status = %s error = %s", event.Status, event.Error)
	}
	if len(fixture.session.Entries()) != 1 {
		t.Fatalf("entries = %#v", fixture.session.Entries())
	}
}

func assertCompactDoesNotCommit(t *testing.T, wantErr string, frames ...string) {
	t.Helper()
	fixture := newCompactFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCompactSSE(w, frames...)
	})
	_, err := fixture.session.Append(session.Message{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	ended := waitRunEnded(t, fixture.events)
	err = fixture.runner.Compact(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	event := <-ended
	if event.Status != RunFailed || !strings.Contains(event.Error, wantErr) {
		t.Fatalf("ended = %#v want %q", event, wantErr)
	}
	if len(fixture.session.Entries()) != 1 {
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
	h := host.NewHost()
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	err = h.Install(&llm.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := host.Resolve[*llm.Client](h, "llm")
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
		Model:           "deepseek/deepseek-v4-flash",
		ReasoningEffort: "off",
		Workspace:       "/workspace/a",
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
