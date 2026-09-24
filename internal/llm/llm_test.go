package llm

import (
	"reflect"
	"testing"

	"github.com/zendev-sh/goai/provider"

	"harness/internal/persist"
	"harness/internal/session"
)

func TestLoadConfig(t *testing.T) {
	config, err := parseConfig([]byte("providers:\n  deepseek:\n    apiKey: secret\n    baseURL: https://example.com\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := config.Providers["deepseek"]
	if got.APIKey != "secret" || got.BaseURL != "https://example.com" {
		t.Fatalf("provider = %#v", got)
	}
}

func TestNewLoadsConfig(t *testing.T) {
	dataDir := t.TempDir()
	files, err := persist.NewFiles(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := files.Write("config.yaml", []byte("providers:\n  deepseek:\n    apiKey: test-key\n")); err != nil {
		t.Fatal(err)
	}

	client, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.Models()) == 0 {
		t.Fatal("no model definitions")
	}
}

func TestParseModelsRequiresContextWindow(t *testing.T) {
	_, err := parseModels([]byte(`{"models":{"x":{"provider":"deepseek","id":"x","reasoning":{"off":{}}}}}`))
	if err == nil {
		t.Fatal("want invalid contextWindow error")
	}
}

func TestParseModelsAllowsMissingVision(t *testing.T) {
	got, err := parseModels([]byte(`{"models":{"x":{"provider":"deepseek","id":"x","contextWindow":100,"reasoning":{"off":{}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	definition := got["x"]
	if definition.ContextWindow != 100 || definition.Vision {
		t.Fatalf("model = %#v", definition)
	}
}

func TestLoadModels(t *testing.T) {
	got, err := loadModels()
	if err != nil {
		t.Fatal(err)
	}
	flash := got["deepseek/deepseek-flash"]
	if flash.Provider != "deepseek" || flash.ID != "deepseek-flash" || flash.ContextWindow != 1000000 || !flash.Vision {
		t.Fatalf("flash = %#v", flash)
	}
	pro := got["deepseek/deepseek-v4-pro"]
	if pro.Provider != "deepseek" || pro.ID != "deepseek-v4-pro" || pro.ContextWindow != 1000000 || pro.Vision {
		t.Fatalf("pro = %#v", pro)
	}
	gemini := got["google/gemini-3.5-flash-lite"]
	if gemini.Provider != "google" || gemini.ID != "gemini-3.5-flash-lite" || gemini.ContextWindow != 1048576 || !gemini.Vision {
		t.Fatalf("gemini = %#v", gemini)
	}
}

func TestContextWindow(t *testing.T) {
	client := &Client{models: map[string]model{
		"deepseek/a": {ContextWindow: 1000},
	}}
	if client.ContextWindow("deepseek/a") != 1000 {
		t.Fatalf("window = %d", client.ContextWindow("deepseek/a"))
	}
	if client.ContextWindow("missing") != 0 {
		t.Fatal("missing model should return 0")
	}
}

func TestVision(t *testing.T) {
	client := &Client{models: map[string]model{
		"sees": {Vision: true},
		"text": {Vision: false},
	}}
	if !client.Vision("sees") || client.Vision("text") || client.Vision("missing") {
		t.Fatalf("vision sees=%v text=%v missing=%v", client.Vision("sees"), client.Vision("text"), client.Vision("missing"))
	}
}

func TestModelsExposesWindowAndVision(t *testing.T) {
	client := &Client{
		config: config{Providers: map[string]providerConfig{"deepseek": {APIKey: "k"}}},
		models: map[string]model{
			"deepseek/a": {Provider: "deepseek", ID: "a", ContextWindow: 1000, Vision: true},
		},
	}
	got := client.Models()
	if len(got) != 1 || got[0].ID != "deepseek/a" || got[0].ContextWindow != 1000 || !got[0].Vision {
		t.Fatalf("models = %#v", got)
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
		{
			Role: session.RoleTool,
			Blocks: []session.Block{{
				Kind: "tool-result",
				Result: &session.ToolResult{
					ID:      "call_1",
					Name:    "search",
					Content: "result",
					IsError: true,
				},
			}},
		},
	}

	got, err := toProviderMessages(history, true)
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
		{Role: provider.RoleTool, Content: []provider.Part{
			{Type: provider.PartToolResult, ToolCallID: "call_1", ToolName: "search", ToolOutput: "result"},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}
}

func TestToProviderMessagesDehydratesImagesWithoutVision(t *testing.T) {
	got, err := toProviderMessages([]session.Message{{
		Role: session.RoleUser,
		Blocks: []session.Block{
			{Kind: "image", Media: &session.Media{MIME: "image/png", Data: "abc"}},
			{Kind: "text", Text: "look"},
		},
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Message{{
		Role: provider.RoleUser,
		Content: []provider.Part{
			{Type: provider.PartText, Text: unrecognizedImageText},
			{Type: provider.PartText, Text: "look"},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}
}

func TestMalformedToolHistoryIsRejected(t *testing.T) {
	cases := []session.Message{
		{Role: session.RoleTool},
		{Role: session.RoleTool, Blocks: []session.Block{{Kind: "text", Text: "missing call id"}}},
	}
	for _, message := range cases {
		if _, err := toProviderMessages([]session.Message{message}, false); err == nil {
			t.Fatal("want tool history error")
		}
	}
}

func TestToolCallWithInvalidArgumentsIsKeptForToolErrorRecovery(t *testing.T) {
	history := []session.Message{{
		Role: session.RoleAssistant,
		Blocks: []session.Block{{
			Kind: "tool-call",
			Tool: &session.ToolCall{ID: "call_1", Name: "search", Args: "not-json"},
		}},
	}}
	messages, err := toProviderMessages(history, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(messages[0].Content[0].ToolInput); got != "not-json" {
		t.Fatalf("ToolInput = %q", got)
	}
}
