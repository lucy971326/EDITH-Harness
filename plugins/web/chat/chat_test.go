package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	"harness/surface/web"
	"harness/surface/web/runview"
	"harness/surface/web/ui"
)

func TestPluginRendersFullPageAndHTMXMain(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()

	client := &nethttp.Client{Timeout: 2 * time.Second}
	response, err := client.Get(webPlugin.URL() + "/chat")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	full := string(body)
	if !strings.Contains(full, "<!doctype html>") || !strings.Contains(full, "创建一个项目开始对话") {
		t.Fatalf("full page = %q", full)
	}
	if strings.Contains(full, "https://") || !strings.Contains(full, "/static/htmx.min.js") {
		t.Fatalf("full page does not use only local assets: %q", full)
	}
	if strings.Count(full, "<aside") != 1 || !strings.Contains(full, `id="product-navigation"`) {
		t.Fatalf("full page does not use one Web sidebar: %q", full)
	}

	request, err := nethttp.NewRequest(nethttp.MethodGet, webPlugin.URL()+"/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("HX-Request", "true")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	partial := string(body)
	if !strings.HasPrefix(partial, "<nav id=\"product-links\" aria-label=\"产品\" class=\"space-y-1\" hx-swap-oob=\"outerHTML\"") ||
		!strings.Contains(partial, "<div id=\"product-navigation\" hx-swap-oob=\"outerHTML\"") ||
		!strings.Contains(partial, "<main id=\"main\"") ||
		strings.Contains(partial, "<!doctype html>") {
		t.Fatalf("HTMX partial = %q", partial)
	}
}

func TestCreateProjectAndCancel(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()

	workspace := t.TempDir()
	previous := selectWorkspace
	selectWorkspace = func(context.Context) (string, error) { return workspace, nil }
	t.Cleanup(func() { selectWorkspace = previous })

	client := &nethttp.Client{Timeout: 2 * time.Second}
	response, err := client.Post(webPlugin.URL()+"/chat/projects", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.Request.URL.Path == "/chat" || !strings.Contains(string(body), `id="composer"`) {
		t.Fatalf("create response path=%q body=%q", response.Request.URL.Path, body)
	}

	selectWorkspace = func(context.Context) (string, error) { return "", ErrCanceled }
	response, err = client.Post(webPlugin.URL()+"/chat/projects", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.Request.URL.Path != "/chat" {
		t.Fatalf("cancel response path = %q", response.Request.URL.Path)
	}
}

func TestCreateProjectFailureDoesNotCreateSession(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()

	previous := selectWorkspace
	selectWorkspace = func(context.Context) (string, error) { return "", errors.New("picker failed") }
	t.Cleanup(func() { selectWorkspace = previous })

	response, err := (&nethttp.Client{Timeout: 2 * time.Second}).Post(webPlugin.URL()+"/chat/projects", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != nethttp.StatusInternalServerError {
		t.Fatalf("status = %d", response.StatusCode)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	list, err := sessions.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("sessions = %#v", list)
	}
}

func TestSettingsFailureDiscardsNewSession(t *testing.T) {
	h := host.NewHost()
	err := h.Install(&persist.Plugin{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&session.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	previous := selectWorkspace
	selectWorkspace = func(context.Context) (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { selectWorkspace = previous })

	handler := &PageHandler{sessions: sessions, settings: failingSettings{}}
	request := httptest.NewRequest(nethttp.MethodPost, "/chat/projects", nil)
	request.Header.Set("HX-Request", "true")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != nethttp.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	list, err := sessions.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("sessions = %#v", list)
	}
}

func TestMessageSelectsModelAndHistoryReturnsLedger(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()
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
	err = settingsStore.Put("session-1", settings.SessionSettings{AgentID: agents.DefaultID, Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"text":            "hello",
		"mode":            "run",
		"model":           "deepseek/deepseek-v4-flash",
		"reasoningEffort": "high",
	} {
		if err := form.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := nethttp.NewRequest(nethttp.MethodPost, webPlugin.URL()+"/chat/session-1/messages", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := nethttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != nethttp.StatusNoContent {
		t.Fatalf("status = %d", response.StatusCode)
	}
	_ = response.Body.Close()

	deadline := time.Now().Add(time.Second)
	for len(sess.History()) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := sess.History(); len(got) != 2 || got[1].Blocks[0].Text != "answer" {
		t.Fatalf("history = %#v", got)
	}
	setup, err := settingsStore.For("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if setup.Model != "deepseek/deepseek-v4-flash" || setup.ReasoningEffort != "high" {
		t.Fatalf("settings = %#v", setup)
	}
	response, err = nethttp.Get(webPlugin.URL() + "/chat/session-1/history")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var history runview.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	if len(history.Entries) != 2 || history.Entries[0].Message.Blocks[0].Text != "hello" || history.Entries[0].Seq != 1 {
		t.Fatalf("history json = %#v", history)
	}
}

func TestSelectModelReplacesSettingsForNextRun(t *testing.T) {
	h, _ := installChat(t)
	defer h.Close()
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		t.Fatal(err)
	}
	if err := settingsStore.Put("session-1", settings.SessionSettings{
		AgentID: agents.DefaultID, Workspace: t.TempDir(), Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high",
	}); err != nil {
		t.Fatal(err)
	}
	models, err := host.Resolve[*llm.Client](h, "llm")
	if err != nil {
		t.Fatal(err)
	}
	handler := &PageHandler{settings: settingsStore, models: models}
	if err := handler.selectModel("session-1", "deepseek/deepseek-v4-pro", "low"); err != nil {
		t.Fatal(err)
	}
	setup, err := settingsStore.For("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if setup.Model != "deepseek/deepseek-v4-pro" || setup.ReasoningEffort != "low" {
		t.Fatalf("settings = %#v", setup)
	}
}

func TestMessageAcceptsURLencodedForm(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Create("session-1"); err != nil {
		t.Fatal(err)
	}
	response, err := nethttp.PostForm(webPlugin.URL()+"/chat/session-1/messages", url.Values{
		"text": {"hello"},
		"mode": {"unknown"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "未知消息模式") {
		t.Fatalf("body = %q", body)
	}
}

func TestPanelRouteRendersRegisteredPanelAndRejectsUnknownType(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Create("session-1"); err != nil {
		t.Fatal(err)
	}
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		t.Fatal(err)
	}
	err = settingsStore.Put("session-1", settings.SessionSettings{AgentID: agents.DefaultID, Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	panels, err := host.Resolve[Service](h, "chat")
	if err != nil {
		t.Fatal(err)
	}
	err = panels.RegisterPanel(testPanel{definition: PanelDefinition{
		ID: "test", Name: "测试", Icon: ui.IconPanel, DefaultInstanceKey: "main", DefaultTabTitle: "测试",
	}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := nethttp.Get(webPlugin.URL() + "/chat/session-1/panels/test?instance=main")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != nethttp.StatusOK || string(body) != "ready" {
		t.Fatalf("panel response = %d %q", response.StatusCode, body)
	}
	response, err = nethttp.Get(webPlugin.URL() + "/chat/session-1/panels/missing?instance=main")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusNotFound {
		t.Fatalf("missing panel status = %d", response.StatusCode)
	}
}

func TestMessageActionReturnsCopyTextAndRejectsInvalidTarget(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := sessions.Create("session-1")
	if err != nil {
		t.Fatal(err)
	}
	question, err := sess.Append(session.Message{
		RunID: "run-1", Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "问题"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sess.Append(session.Message{
		RunID: "run-1", Role: session.RoleAssistant, Blocks: []session.Block{
			{Kind: "reasoning", Text: "不复制"}, {Kind: "text", Text: "回答"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := nethttp.PostForm(webPlugin.URL()+"/chat/session-1/message-actions/copy", url.Values{
		"cardType": {"assistant"}, "runID": {"run-1"}, "boundaryEntryID": {question.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusOK {
		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		t.Fatalf("status = %d body=%q", response.StatusCode, body)
	}
	var result MessageActionResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Text != "回答" {
		t.Fatalf("copy result = %#v", result)
	}

	response, err = nethttp.PostForm(webPlugin.URL()+"/chat/session-1/message-actions/copy", url.Values{
		"cardType": {"user"}, "entryID": {"missing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("invalid target status = %d", response.StatusCode)
	}

	response, err = nethttp.PostForm(webPlugin.URL()+"/chat/session-1/message-actions/missing", url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusNotFound {
		t.Fatalf("missing action status = %d", response.StatusCode)
	}
}

type failingSettings struct{}

func (failingSettings) For(string) (settings.SessionSettings, error) {
	return settings.SessionSettings{}, errors.New("unavailable")
}

func (failingSettings) Put(string, settings.SessionSettings) error {
	return errors.New("disk full")
}

func installChat(t *testing.T) (*host.Host, *web.Plugin) {
	t.Helper()
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".harness"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, ".harness", "config.yaml")
	if err := os.WriteFile(configPath, []byte("providers:\n  deepseek:\n    apiKey: test-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	previousHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", previousHome)
	})
	h := host.NewHost()
	err := h.Install(&persist.Plugin{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&session.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&llm.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(events.NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(loops.NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	loopRegistry, err := host.Resolve[loops.Loops](h, "loops")
	if err != nil {
		t.Fatal(err)
	}
	err = loopRegistry.Register(chatTestLoop{})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(skills.NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(tools.NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(agents.NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(runner.NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	webPlugin := web.NewPlugin(web.Config{Host: "127.0.0.1", Port: 0})
	err = h.Install(webPlugin)
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	return h, webPlugin
}

type chatTestLoop struct{}

func (chatTestLoop) Definition() loops.Definition {
	return loops.Definition{Kind: "react", Description: "chat test"}
}

func (chatTestLoop) Run(ctx context.Context, invocation loops.Invocation) error {
	message := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "answer"}}}
	return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, Message: &message})
}
