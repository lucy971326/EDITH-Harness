package llm

import (
	"os"
	"reflect"
	"testing"

	"github.com/zendev-sh/goai/provider"

	"harness/kernel/host"
	"harness/kernel/session"
)

func TestLoadConfig(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte("providers:\n  deepseek:\n    apiKey: secret\n    baseURL: https://example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := readConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	got := config.Providers["deepseek"]
	if got.APIKey != "secret" || got.BaseURL != "https://example.com" {
		t.Fatalf("provider = %#v", got)
	}
}

func TestPluginStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(home+"/.harness", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home+"/.harness/config.yaml", []byte("providers:\n  deepseek:\n    apiKey: test-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := host.NewHost()
	plugin := &Plugin{}
	if err := h.Install(plugin); err != nil {
		t.Fatal(err)
	}
	client, err := host.Resolve[*Client](h, "llm")
	if err != nil {
		t.Fatal(err)
	}
	if client != plugin.client {
		t.Fatal("registered client differs from plugin client")
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if plugin.client != nil {
		t.Fatal("client remains after close")
	}
}

func TestReasoningOptions(t *testing.T) {
	definition := model{ID: "chat", Reasoning: map[string]map[string]any{
		"off": {"thinking": map[string]any{"type": "disabled"}},
	}}

	got, err := reasoningOptions(definition, "off")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"thinking": map[string]any{"type": "disabled"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
	if _, err := reasoningOptions(definition, "high"); err == nil {
		t.Fatal("want unsupported effort error")
	}
}

func TestToProviderMessages(t *testing.T) {
	history := []session.Message{
		{
			Role: session.RoleUser,
			Blocks: []session.Block{
				{Kind: "text", Text: "look"},
				{Kind: "image", Media: &session.Media{MIME: "image/png", Data: "abc"}},
			},
		},
		{
			Role: session.RoleAssistant,
			Blocks: []session.Block{
				{Kind: "reasoning", Text: "think"},
				{Kind: "text", Text: "answer"},
				{Kind: "tool-call", Tool: &session.ToolCall{ID: "call_1", Name: "search", Args: `{"q":"go"}`}},
			},
		},
	}

	got, err := toProviderMessages(history)
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Message{
		{Role: provider.RoleUser, Content: []provider.Part{
			{Type: provider.PartText, Text: "look"},
			{Type: provider.PartImage, URL: "data:image/png;base64,abc", MediaType: "image/png"},
		}},
		{Role: provider.RoleAssistant, Content: []provider.Part{
			{Type: provider.PartReasoning, Text: "think"},
			{Type: provider.PartText, Text: "answer"},
			{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "search", ToolInput: []byte(`{"q":"go"}`)},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}
}

func TestToolHistoryIsRejected(t *testing.T) {
	_, err := toProviderMessages([]session.Message{{Role: session.RoleTool}})
	if err == nil {
		t.Fatal("want tool history error")
	}
}
