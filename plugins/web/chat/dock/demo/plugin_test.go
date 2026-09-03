package demo

import (
	"bufio"
	"context"
	"io"
	nethttp "net/http"
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
	"harness/plugins/web/chat"
	"harness/surface/web"
)

func TestPluginStreamsUpdatedDockHTML(t *testing.T) {
	h, webPlugin, plugin := installChatWithDemo(t)
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
	if err := settingsStore.Put("session-1", settings.SessionSettings{AgentID: agents.DefaultID, Workspace: t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	client := &nethttp.Client{Timeout: 2 * time.Second}
	page, err := client.Get(webPlugin.URL() + "/chat/session-1")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(page.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `sse-swap="dock-demo"`) || !strings.Contains(string(body), "演示计数：0") {
		t.Fatalf("page does not render dock: %s", body)
	}

	stream, err := client.Get(webPlugin.URL() + "/chat/session-1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	reader := bufio.NewReader(stream.Body)
	event, data, err := readSSEEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if event != "dock-demo" || !strings.Contains(data, "演示计数：0") {
		t.Fatalf("initial event=%q data=%q", event, data)
	}
	if err := plugin.Increment(context.Background(), "session-1"); err != nil {
		t.Fatal(err)
	}
	event, data, err = readSSEEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if event != "dock-demo" || !strings.Contains(data, "演示计数：1") {
		t.Fatalf("updated event=%q data=%q", event, data)
	}
}

func readSSEEvent(reader *bufio.Reader) (string, string, error) {
	var event string
	var data []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", "", err
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			if event != "" {
				return event, strings.Join(data, "\n"), nil
			}
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = append(data, strings.TrimPrefix(line, "data: "))
		}
	}
}

func installChatWithDemo(t *testing.T) (*host.Host, *web.Plugin, *Plugin) {
	t.Helper()
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".harness"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".harness", "config.yaml"), []byte("providers:\n  deepseek:\n    apiKey: test-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	previousHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("HOME", previousHome) })

	h := host.NewHost()
	for _, plugin := range []host.Plugin{
		&persist.Plugin{Dir: t.TempDir()},
		&session.Plugin{},
		&llm.Plugin{},
		events.NewPlugin(),
		loops.NewPlugin(),
		skills.NewPlugin(),
		tools.NewPlugin(),
		agents.NewPlugin(),
		runner.NewPlugin(),
	} {
		if err := h.Install(plugin); err != nil {
			t.Fatal(err)
		}
	}
	webPlugin := web.NewPlugin(web.Config{Host: "127.0.0.1", Port: 0})
	if err := h.Install(webPlugin); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(chat.NewPlugin()); err != nil {
		t.Fatal(err)
	}
	plugin := New()
	if err := h.Install(plugin); err != nil {
		t.Fatal(err)
	}
	return h, webPlugin, plugin
}
