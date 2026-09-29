package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/zendev-sh/goai/provider"

	"harness/internal/persist"
	"harness/internal/session"
	"harness/internal/tools"
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
	if off, err := reasoningOptions(gemini, "off"); err != nil || off["google"].(map[string]any)["thinkingConfig"] != false {
		t.Fatalf("gemini off = %#v, %v", off, err)
	}
}

func TestContextWindow(t *testing.T) {
	client := newTestClientState(modelState{models: map[string]model{
		"deepseek/a": {ContextWindow: 1000},
	}})
	if client.ContextWindow("deepseek/a") != 1000 {
		t.Fatalf("window = %d", client.ContextWindow("deepseek/a"))
	}
	if client.ContextWindow("missing") != 0 {
		t.Fatal("missing model should return 0")
	}
}

func TestVision(t *testing.T) {
	client := newTestClientState(modelState{models: map[string]model{
		"sees": {Vision: true},
		"text": {Vision: false},
	}})
	if !client.Vision("sees") || client.Vision("text") || client.Vision("missing") {
		t.Fatalf("vision sees=%v text=%v missing=%v", client.Vision("sees"), client.Vision("text"), client.Vision("missing"))
	}
}

func TestModelsExposesWindowAndVision(t *testing.T) {
	client := newTestClientState(modelState{
		config: config{Providers: map[string]providerConfig{"deepseek": {APIKey: "k"}}},
		models: map[string]model{
			"deepseek/a": {Provider: "deepseek", ID: "a", ContextWindow: 1000, Vision: true},
		},
	})
	got := client.Models()
	if len(got) != 1 || got[0].ID != "deepseek/a" || got[0].Provider != "deepseek" || got[0].ContextWindow != 1000 || !got[0].Vision {
		t.Fatalf("models = %#v", got)
	}
}

func TestReasoningOptions(t *testing.T) {
	definition := model{ID: "chat", Reasoning: reasoningLevels{
		{Effort: "off", Options: map[string]any{"thinking": map[string]any{"type": "disabled"}}},
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

func TestModelsPreservesReasoningOrder(t *testing.T) {
	for _, test := range []struct {
		name      string
		reasoning string
		want      []string
	}{
		{"configured", `{"off":{},"low":{},"high":{},"max":{}}`, []string{"off", "low", "high", "max"}},
		{"custom", `{"quick":{},"balanced":{},"deep":{}}`, []string{"quick", "balanced", "deep"}},
		{"single", `{"off":{}}`, []string{"off"}},
		{"empty", `{}`, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			models, err := parseModels([]byte(`{"models":{"x":{"provider":"deepseek","id":"x","contextWindow":100,"reasoning":` + test.reasoning + `}}}`))
			if err != nil {
				t.Fatal(err)
			}
			client := newTestClientState(modelState{config: config{Providers: map[string]providerConfig{"deepseek": {APIKey: "k"}}}, models: models})
			got := client.Models()[0].ReasoningEfforts
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("efforts = %v, want %v", got, test.want)
			}
			for _, effort := range got {
				if _, err := reasoningOptions(models["x"], effort); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	for _, reasoning := range []string{`[]`, `{"off":{},"off":{}}`, `{"off":42}`} {
		_, err := parseModels([]byte(`{"models":{"x":{"contextWindow":100,"reasoning":` + reasoning + `}}}`))
		if err == nil {
			t.Fatalf("want error for reasoning %s", reasoning)
		}
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

// 首次启动、密钥更新与跨文件版本是配置界面的真实恢复边界。
func TestSettingsFirstRunAndSecrets(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.Read("models.json"); err != nil {
		t.Fatal(err)
	}
	view, err := client.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Models) == 0 || len(client.Models()) != 0 {
		t.Fatalf("first run: view=%d choices=%d", len(view.Models), len(client.Models()))
	}
	provider, err := client.SaveProvider(SaveProviderInput{ID: "deepseek", Protocol: "deepseek", APIKey: "secret-one", Revision: view.ProviderRevision})
	if err != nil {
		t.Fatal(err)
	}
	if !provider.Providers[0].HasAPIKey {
		t.Fatal("key status not set")
	}
	serialized, err := json.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "secret-one") {
		t.Fatal("read result leaked key")
	}
	if len(client.Models()) == 0 {
		t.Fatal("saved provider did not publish models")
	}
	_, err = client.SaveProvider(SaveProviderInput{ID: "deepseek", Protocol: "deepseek", APIKey: "stale", Revision: view.ProviderRevision})
	if !errors.Is(err, ErrSettingsChanged) {
		t.Fatalf("stale revision: %v", err)
	}
	if err := files.Write("config.yaml", []byte("jev:\n  apiKey: jev-secret\nproviders:\n  deepseek:\n    protocol: deepseek\n    apiKey: secret-one\n")); err != nil {
		t.Fatal(err)
	}
	provider, err = client.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	provider, err = client.SaveProvider(SaveProviderInput{ID: "deepseek", Protocol: "deepseek", APIKey: "secret-two", Revision: provider.ProviderRevision})
	if err != nil {
		t.Fatal(err)
	}
	configBody, err := files.Read("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configBody), "jev-secret") || !strings.Contains(string(configBody), "secret-two") || strings.Contains(string(configBody), "secret-one") {
		t.Fatalf("unexpected config: %s", configBody)
	}
	provider, err = client.SaveProvider(SaveProviderInput{ID: "deepseek", Protocol: "deepseek", ClearAPIKey: true, Revision: provider.ProviderRevision})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Providers[0].HasAPIKey || len(client.Models()) != 0 {
		t.Fatal("key clear did not update choices")
	}
}

func TestSettingsModelsOrderDeleteAndPin(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	view, err := client.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	view, err = client.SaveProvider(SaveProviderInput{ID: "custom", Protocol: "openai-chat", APIKey: "first", Revision: view.ProviderRevision})
	if err != nil {
		t.Fatal(err)
	}
	modelInput := ModelSettings{Provider: "custom", ID: "model-a", ContextWindow: 4096, Vision: true,
		Reasoning: []ReasoningSettings{{Name: "off", Mode: "off"}, {Name: "low", Mode: "enabled", Effort: "low"}, {Name: "high", Mode: "enabled", Effort: "high"}}}
	view, err = client.SaveModel(SaveModelInput{Model: modelInput, Revision: view.ModelRevision})
	if err != nil {
		t.Fatal(err)
	}
	pinned := client.Pin()
	choices := client.Models()
	if len(choices) != 1 || !reflect.DeepEqual(choices[0].ReasoningEfforts, []string{"off", "low", "high"}) {
		t.Fatalf("choices: %+v", choices)
	}
	body, err := files.Read("models.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(body), `"off"`) > strings.Index(string(body), `"low"`) || strings.Index(string(body), `"low"`) > strings.Index(string(body), `"high"`) {
		t.Fatal("reasoning order changed on disk")
	}
	_, err = client.SaveProvider(SaveProviderInput{ID: "custom", Protocol: "anthropic", Revision: view.ProviderRevision})
	if !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("protocol changed with models: %v", err)
	}
	view, err = client.DeleteModel("custom/model-a", view.ModelRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.Models()) != 0 || len(pinned.Models()) != 1 {
		t.Fatal("pin or deletion failed")
	}
	_, err = client.DeleteModel("custom/model-a", view.ModelRevision)
	if !errors.Is(err, ErrSettingsMissing) {
		t.Fatalf("delete missing: %v", err)
	}
	view, err = client.DeleteProvider("custom", view.ProviderRevision, view.ModelRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Models) != 4 {
		t.Fatalf("built-in models lost: %d", len(view.Models))
	}
}

func TestSettingsInvalidInputAndExternalChange(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	view, err := client.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SaveProvider(SaveProviderInput{ID: "bad/id", Protocol: "openai-chat", Revision: view.ProviderRevision})
	if !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("bad id: %v", err)
	}
	_, err = client.SaveProvider(SaveProviderInput{ID: "test", Protocol: "openai-chat", BaseURL: "file:///tmp", Revision: view.ProviderRevision})
	if !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("bad URL: %v", err)
	}
	if err := files.Write("models.json", []byte(`{"models":{}}`)); err != nil {
		t.Fatal(err)
	}
	_, err = client.DeleteModel("deepseek/deepseek-flash", view.ModelRevision)
	if !errors.Is(err, ErrSettingsChanged) {
		t.Fatalf("external edit: %v", err)
	}
	view, err = client.ReadSettings()
	if err != nil || len(view.Models) != 0 {
		t.Fatalf("reload disk: %v, %+v", err, view.Models)
	}
	// 磁盘数据损坏时不把内存目录当作已保存的新配置。
	if err := files.Write("models.json", []byte("not json")); err != nil {
		t.Fatal(err)
	}
	_, err = client.ReadSettings()
	if err == nil {
		t.Fatal("want disk parse error")
	}
	_, err = client.SaveProvider(SaveProviderInput{ID: "test", Protocol: "openai-chat", Revision: view.ProviderRevision})
	if err == nil {
		t.Fatal("save must fail while on-disk models are invalid")
	}
	if len(client.Models()) != 0 {
		t.Fatal("invalid disk data became an active model")
	}
}

// 五条 API Key 接入路径都必须把 UI 档位、鉴权和工具定义交给对应的流协议。
func TestConfiguredProtocolsStreamToolCalls(t *testing.T) {
	chatSSE := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"tc1","type":"function","function":{"name":"read_file","arguments":"{}"}}]},"index":0}]}` + "\n\n" +
		`data: {"choices":[{"delta":{},"index":0,"finish_reason":"tool_calls"}]}` + "\n\n" + "data: [DONE]\n\n"
	responsesSSE := "event: response.output_item.added\n" +
		`data: {"output_index":0,"item":{"type":"function_call","call_id":"tc1","name":"read_file"}}` + "\n\n" +
		"event: response.function_call_arguments.delta\n" + `data: {"output_index":0,"delta":"{}"}` + "\n\n" +
		"event: response.function_call_arguments.done\n" + `data: {"output_index":0,"arguments":"{}"}` + "\n\n" +
		"event: response.output_item.done\n" + `data: {"output_index":0}` + "\n\n" +
		"event: response.completed\n" + `data: {"response":{"usage":{"input_tokens":2,"output_tokens":3}}}` + "\n\n"
	googleSSE := `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"read_file","args":{}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}` + "\n\n"
	anthropicSSE := `data: {"type":"message_start","message":{"usage":{"input_tokens":2}}}` + "\n\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tc1","name":"read_file"}}` + "\n\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}` + "\n\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}` + "\n\n" +
		`data: {"type":"message_stop"}` + "\n\n"
	for _, test := range []struct {
		protocol, path, authHeader, sse string
	}{
		{"deepseek", "/chat/completions", "Authorization", chatSSE},
		{"google", ":streamGenerateContent", "x-goog-api-key", googleSSE},
		{"openai-chat", "/chat/completions", "Authorization", chatSSE},
		{"openai-responses", "/responses", "Authorization", responsesSSE},
		{"anthropic", "/v1/messages", "x-api-key", anthropicSSE},
	} {
		t.Run(test.protocol, func(t *testing.T) {
			requests := make(chan struct {
				path, auth string
				body       map[string]any
			}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				body, _ := io.ReadAll(request.Body)
				var decoded map[string]any
				_ = json.Unmarshal(body, &decoded)
				requests <- struct {
					path, auth string
					body       map[string]any
				}{request.URL.Path, request.Header.Get(test.authHeader), decoded}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, test.sse)
			}))
			defer server.Close()
			setting := ModelSettings{Provider: "custom", ID: "model-a", ContextWindow: 4096,
				Reasoning: []ReasoningSettings{{Name: "low", Mode: "enabled", Effort: "low", BudgetTokens: 1024}}}
			_, definition, err := modelFromSettings(setting, test.protocol)
			if err != nil {
				t.Fatal(err)
			}
			client := newTestClientState(modelState{config: config{Providers: map[string]providerConfig{"custom": {
				Protocol: test.protocol, APIKey: "test-key", BaseURL: server.URL,
			}}}, models: map[string]model{"custom/model-a": definition}})
			stream, err := client.Stream(t.Context(), RunConfig{Model: "custom/model-a", ReasoningEffort: "low"}, Input{
				History: []session.Message{{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "read"}}}},
				Tools:   []tools.Definition{{Name: "read_file", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
			})
			if err != nil {
				t.Fatal(err)
			}
			call := false
			for chunk := range stream {
				if chunk.Type == provider.ChunkError {
					t.Fatalf("stream error: %v", chunk.Error)
				}
				if chunk.Type == provider.ChunkToolCall && chunk.ToolName == "read_file" {
					call = true
				}
			}
			if !call {
				t.Fatal("stream did not surface the tool call")
			}
			request := <-requests
			if !strings.Contains(request.path, test.path) {
				t.Fatalf("path = %q", request.path)
			}
			if !strings.Contains(request.auth, "test-key") {
				t.Fatalf("auth header missing: %q", request.auth)
			}
			if _, ok := request.body["tools"]; !ok {
				t.Fatalf("request missing tools: %+v", request.body)
			}
			switch test.protocol {
			case "deepseek", "openai-chat":
				if request.body["reasoning_effort"] != "low" {
					t.Fatalf("reasoning: %+v", request.body)
				}
			case "openai-responses":
				if request.body["reasoning"].(map[string]any)["effort"] != "low" {
					t.Fatalf("reasoning: %+v", request.body)
				}
			case "google":
				config := request.body["generationConfig"].(map[string]any)["thinkingConfig"].(map[string]any)
				if config["thinkingLevel"] != "low" {
					t.Fatalf("thinkingConfig: %+v", config)
				}
			case "anthropic":
				if request.body["thinking"].(map[string]any)["budget_tokens"] != float64(1024) {
					t.Fatalf("thinking: %+v", request.body)
				}
			}
		})
	}
}

func TestReasoningOffByProtocol(t *testing.T) {
	for _, protocol := range []string{"deepseek", "google", "openai-chat", "openai-responses", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			options, err := levelOptions(ReasoningSettings{Name: "off", Mode: "off"}, protocol)
			if err != nil {
				t.Fatal(err)
			}
			if got := levelSettings(reasoningLevel{Effort: "off", Options: options}, protocol); got.Mode != "off" {
				t.Fatalf("off became %q", got.Mode)
			}
			if protocol == "google" {
				google := options["google"].(map[string]any)
				if google["thinkingConfig"] != false {
					t.Fatalf("Google off request: %+v", options)
				}
			}
		})
	}
}

func TestSettingsCanRecoverAfterLastModelRemoved(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	view, err := client.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	for len(view.Models) > 0 {
		view, err = client.DeleteModel(view.Models[0].Key, view.ModelRevision)
		if err != nil {
			t.Fatal(err)
		}
	}
	restarted, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	view, err = restarted.ReadSettings()
	if err != nil || len(view.Models) != 0 || len(view.Presets) != 4 {
		t.Fatalf("empty catalog cannot reopen with presets: models=%d presets=%d, %v", len(view.Models), len(view.Presets), err)
	}
	view, err = restarted.SaveProvider(SaveProviderInput{ID: "openai", Protocol: "openai-chat", APIKey: "key", Revision: view.ProviderRevision})
	if err != nil {
		t.Fatal(err)
	}
	_, err = restarted.SaveModel(SaveModelInput{Revision: view.ModelRevision, Model: ModelSettings{
		Provider: "openai", ID: "example", ContextWindow: 4096,
		Reasoning: []ReasoningSettings{{Name: "off", Mode: "off"}},
	}})
	if err != nil || len(restarted.Models()) != 1 {
		t.Fatalf("readd after empty catalog: %v", err)
	}
}

func newTestClientState(state modelState) *Client {
	client := &Client{}
	client.current.Store(&state)
	return client
}
