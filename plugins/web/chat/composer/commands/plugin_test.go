package commands

import (
	"context"
	"io"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harness/kernel/agents"
	chatservice "harness/kernel/chat"
	kerncommands "harness/kernel/commands"
	"harness/kernel/events"
	"harness/kernel/host"
	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
	"harness/kernel/subagents"
	"harness/kernel/tools"
	compactcmd "harness/plugins/kernel/commands/compact"
	chat "harness/plugins/web/chat"
	"harness/surface/web"
)

func TestSlashListsCompactAndRejectsEmptyCompact(t *testing.T) {
	h, url := installChatWithCommands(t)
	defer h.Close()

	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	_, err = sessions.Create("cmd-session")
	if err != nil {
		t.Fatal(err)
	}
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		t.Fatal(err)
	}
	err = settingsStore.Put("cmd-session", settings.SessionSettings{
		AgentID:         agents.DefaultID,
		Model:           "deepseek/deepseek-v4-flash",
		ReasoningEffort: "off",
		Workspace:       t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	client := &nethttp.Client{Timeout: 2 * time.Second}
	response, err := client.Get(url + "/chat/cmd-session/suggestions?prefix=/")
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
	html := string(body)
	if !strings.Contains(html, "命令") || !strings.Contains(html, `data-suggestion-name="compact"`) {
		t.Fatalf("suggestions = %q", html)
	}

	response, err = client.Post(url+"/chat/cmd-session/commands/compact", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("empty compact status = %d", response.StatusCode)
	}
}

func installChatWithCommands(t *testing.T) (*host.Host, string) {
	t.Helper()
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".harness"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".harness", "config.yaml"), []byte("providers:\n  deepseek:\n    apiKey: test-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

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
		kerncommands.NewPlugin(),
		runner.NewPlugin(),
		subagents.NewPlugin(home),
		chatservice.NewPlugin(),
		compactcmd.New(),
	} {
		if err := h.Install(plugin); err != nil {
			t.Fatal(err)
		}
	}
	loopRegistry, err := host.Resolve[loops.Loops](h, "loops")
	if err != nil {
		t.Fatal(err)
	}
	err = loopRegistry.Register(testLoop{})
	if err != nil {
		t.Fatal(err)
	}
	webPlugin := web.NewPlugin(web.Config{Host: "127.0.0.1", Port: 0})
	if err := h.Install(webPlugin); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(chat.NewPlugin()); err != nil {
		t.Fatal(err)
	}
	if err := h.Install(New()); err != nil {
		t.Fatal(err)
	}
	return h, webPlugin.URL()
}

type testLoop struct{}

func (testLoop) Definition() loops.Definition {
	return loops.Definition{Kind: "react", Description: "test"}
}

func (testLoop) Run(context.Context, loops.Invocation) error { return nil }
