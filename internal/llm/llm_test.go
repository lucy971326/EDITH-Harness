package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestContextBudgetAndEstimate(t *testing.T) {
	client := &Client{}
	client.current.Store(&modelState{models: map[string]model{"test": {ContextWindow: 100000, MaxOutput: 10000}}})
	if client.InputBudget("test") != 80000 || client.InputBudget("missing") != 0 {
		t.Fatal("wrong request budget")
	}
	if client.MaxOutput("test") != 10000 || client.MaxOutput("missing") != 0 {
		t.Fatal("wrong model output limit")
	}
	base := EstimateInput(Input{History: []session.Message{{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "hello"}}}}})
	withImage := EstimateInput(Input{System: "rules", History: []session.Message{{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "hello"}, {Kind: "image", Media: &session.Media{MIME: "image/png", Data: "base64"}}}}}})
	if withImage <= base+4096 {
		t.Fatal("missing fixed context/media estimate")
	}
	for _, tc := range []struct {
		code, text string
		overflow   bool
	}{
		{"context_length_exceeded", "too big", true},
		{"invalid_request_error", "prompt is too long: 10000 tokens", true},
		{"authentication_error", "invalid key", false},
		{"rate_limit_exceeded", "token rate limit", false},
	} {
		if errors.Is(contextError(tc.code, tc.text), ErrContextWindow) != tc.overflow {
			t.Fatalf("wrong classification: %+v", tc)
		}
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
	_, err := parseModels([]byte(`{"version":2,"models":{"x":{"provider":"deepseek","id":"x","reasoning":{"off":{}}}}}`))
	if err == nil {
		t.Fatal("want invalid contextWindow error")
	}
}

func TestParseModelsAllowsMissingVision(t *testing.T) {
	got, err := parseModels([]byte(`{"version":2,"models":{"x":{"provider":"deepseek","id":"x","contextWindow":100,"reasoning":{"off":{}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	definition := got["x"]
	if definition.ContextWindow != 100 || definition.Vision {
		t.Fatalf("model = %#v", definition)
	}
}

func TestLoadModels(t *testing.T) {
	models, err := loadModels()
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"deepseek", "xiaomi", "minimax", "qwen-token-plan", "moonshotai", "zai", "baseten", "cerebras", "fireworks", "groq", "nvidia", "opencode", "github-copilot", "cloudflare-ai-gateway", "openai-codex", "xai-oauth"} {
		count := 0
		for _, model := range models {
			if model.Provider == provider {
				count++
				if model.MaxOutput <= 0 || model.ContextWindow <= 0 || len(model.Reasoning) == 0 {
					t.Fatalf("invalid catalog model: %+v", model)
				}
			}
		}
		if count == 0 {
			t.Fatalf("provider missing: %s", provider)
		}
	}
	for _, level := range models["xiaomi/mimo-v2.6-pro"].Reasoning {
		if level.Effort != "off" && level.Effort != "on" {
			t.Fatal("invented MiMo reasoning strength")
		}
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
			models, err := parseModels([]byte(`{"version":2,"models":{"x":{"provider":"deepseek","id":"x","contextWindow":100,"reasoning":` + test.reasoning + `}}}`))
			if err != nil {
				t.Fatal(err)
			}
			client := newTestClientState(modelState{config: config{Providers: map[string]providerConfig{"deepseek": {APIKey: "k"}}}, models: map[string]model{"x": models["x"]}})
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
		_, err := parseModels([]byte(`{"version":2,"models":{"x":{"contextWindow":100,"reasoning":` + reasoning + `}}}`))
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
	want := []modelMessage{
		{Role: RoleUser, Content: []part{
			{Type: PartText, Text: "look"},
			{Type: PartImage, URL: "data:image/png;base64,abc", MediaType: "image/png"},
		}},
		{Role: RoleAssistant, Content: []part{
			{Type: PartReasoning, Text: "think"},
			{Type: PartText, Text: "answer"},
			{Type: PartToolCall, ToolCallID: "call_1", ToolName: "search", ToolInput: []byte(`{"q":"go"}`)},
		}},
		{Role: RoleTool, Content: []part{
			{Type: PartToolResult, ToolCallID: "call_1", ToolName: "search", ToolOutput: "result", IsError: true},
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
	want := []modelMessage{{
		Role: RoleUser,
		Content: []part{
			{Type: PartText, Text: unrecognizedImageText},
			{Type: PartText, Text: "look"},
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

func TestToolCallWithInvalidArgumentsCanBeReplayed(t *testing.T) {
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
	if got := string(messages[0].Content[0].ToolInput); got != "{}" {
		t.Fatalf("ToolInput = %q", got)
	}
	if history[0].Blocks[0].Tool.Args != "not-json" {
		t.Fatal("original tool arguments changed")
	}
	if _, err := json.Marshal((streamRequest{messages: messages}).anthropicBody()); err != nil {
		t.Fatalf("Anthropic replay failed: %v", err)
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
	provider, err := client.SaveProvider(SaveProviderInput{ID: "deepseek", Protocol: "openai-chat", APIKey: "secret-one", Revision: view.ProviderRevision})
	if err != nil {
		t.Fatal(err)
	}
	if !providerByID(provider, "deepseek").HasAPIKey {
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
	_, err = client.SaveProvider(SaveProviderInput{ID: "deepseek", Protocol: "openai-chat", APIKey: "stale", Revision: view.ProviderRevision})
	if !errors.Is(err, ErrSettingsChanged) {
		t.Fatalf("stale revision: %v", err)
	}
	if err := files.Write("config.yaml", []byte("jev:\n  apiKey: jev-secret\nproviders:\n  deepseek:\n    protocol: openai-chat\n    apiKey: secret-one\n")); err != nil {
		t.Fatal(err)
	}
	provider, err = client.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	provider, err = client.SaveProvider(SaveProviderInput{ID: "deepseek", Protocol: "openai-chat", APIKey: "secret-two", Revision: provider.ProviderRevision})
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
	provider, err = client.SaveProvider(SaveProviderInput{ID: "deepseek", Protocol: "openai-chat", ClearAPIKey: true, Revision: provider.ProviderRevision})
	if err != nil {
		t.Fatal(err)
	}
	if providerByID(provider, "deepseek").HasAPIKey || len(client.Models()) != 0 {
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
	modelInput := ModelSettings{Provider: "custom", Protocol: "openai-responses", ID: "model-a", ContextWindow: 4096, Vision: true,
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
	if !strings.Contains(string(body), `"protocol": "openai-responses"`) {
		t.Fatalf("model protocol was not saved: %s", body)
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
	defaults, err := loadModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Models) != len(defaults) {
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
	if err := files.Write("models.json", []byte(`{"version":2,"models":{},"hidden":["deepseek/deepseek-flash"]}`)); err != nil {
		t.Fatal(err)
	}
	_, err = client.DeleteModel("deepseek/deepseek-flash", view.ModelRevision)
	if !errors.Is(err, ErrSettingsChanged) {
		t.Fatalf("external edit: %v", err)
	}
	view, err = client.ReadSettings()
	if err != nil {
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

// 三类流协议和兼容供应商都必须把 UI 档位、鉴权和工具定义交给对应的流协议。
func TestConfiguredProtocolsStreamToolCalls(t *testing.T) {
	chatSSE := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"tc1","type":"function","function":{"name":"read_file","arguments":"{}"}}]},"index":0}]}` + "\n\n" +
		`data: {"choices":[{"delta":{},"index":0,"finish_reason":"tool_calls"}]}` + "\n\n" + "data: [DONE]\n\n"
	responsesSSE := "event: response.output_item.added\n" +
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"tc1","name":"read_file"}}` + "\n\n" +
		"event: response.function_call_arguments.delta\n" + `data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{}"}` + "\n\n" +
		"event: response.function_call_arguments.done\n" + `data: {"type":"response.function_call_arguments.done","output_index":0,"arguments":"{}"}` + "\n\n" +
		"event: response.output_item.done\n" + `data: {"type":"response.output_item.done","output_index":0}` + "\n\n" +
		"event: response.completed\n" + `data: {"type":"response.completed","response":{"usage":{"input_tokens":2,"output_tokens":3}}}` + "\n\n"
	anthropicSSE := "event: message_start\n" + `data: {"type":"message_start","message":{"usage":{"input_tokens":2}}}` + "\n\n" +
		"event: content_block_start\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tc1","name":"read_file"}}` + "\n\n" +
		"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}` + "\n\n" +
		"event: content_block_stop\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}` + "\n\n" +
		"event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"
	for _, test := range []struct {
		protocol, path, authHeader, sse string
	}{
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
				if chunk.Type == ChunkError {
					t.Fatalf("stream error: %v", chunk.Error)
				}
				if chunk.Type == ChunkToolCall && chunk.ToolName == "read_file" {
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
			case "anthropic":
				if request.body["thinking"].(map[string]any)["budget_tokens"] != float64(1024) {
					t.Fatalf("thinking: %+v", request.body)
				}
			}
		})
	}
}

func TestReasoningOffByProtocol(t *testing.T) {
	for _, protocol := range []string{"deepseek", "qwen", "openrouter", "openai-chat", "openai-responses", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			options, err := levelOptions(ReasoningSettings{Name: "off", Mode: "off"}, protocol)
			if err != nil {
				t.Fatal(err)
			}
			if got := levelSettings(reasoningLevel{Effort: "off", Options: options}, protocol); got.Mode != "off" {
				t.Fatalf("off became %q", got.Mode)
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
	empty, err := encodeModels(map[string]model{})
	if err != nil {
		t.Fatal(err)
	}
	if err := files.Write("models.json", empty); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	view, err = restarted.ReadSettings()
	defaults, loadErr := loadModels()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if err != nil || len(view.Models) != 0 || len(view.Presets) != len(defaults) {
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

func providerByID(view SettingsView, id string) ProviderSettings {
	for _, provider := range view.Providers {
		if provider.ID == id {
			return provider
		}
	}
	return ProviderSettings{}
}

func TestModelsRejectUnsupportedFormat(t *testing.T) {
	for _, body := range []string{`{"models":{}}`, `{"version":1,"models":{}}`, `{"version":3,"models":{}}`} {
		if _, err := parseModels([]byte(body)); err == nil {
			t.Fatal("unsupported format accepted")
		}
	}
}

func TestCatalogGeneratedValuesDoNotBecomeUserOverrides(t *testing.T) {
	models, err := loadModels()
	if err != nil {
		t.Fatal(err)
	}
	body, err := encodeModels(models)
	if err != nil {
		t.Fatal(err)
	}
	var file modelFile
	if err := json.Unmarshal(body, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Models) != 0 || len(file.Hidden) != 0 {
		t.Fatal("builtins copied to user config")
	}
}

func TestStreamRejectsTruncatedCompletion(t *testing.T) {
	for _, protocol := range []string{"openai-chat", "openai-responses", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {}\n\n")
			}))
			defer server.Close()
			failed := false
			for event := range startStream(t.Context(), streamRequest{protocol: protocol, baseURL: server.URL, token: "test", definition: model{ID: "test", ContextWindow: 4096}}) {
				if event.Type == ChunkError {
					failed = true
				}
				if event.Type == ChunkFinish {
					t.Fatal("truncated stream marked complete")
				}
			}
			if !failed {
				t.Fatal("missing interruption error")
			}
		})
	}
}

func TestStreamCancellationClosesHTTPRequest(t *testing.T) {
	connected := make(chan struct{})
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {}\n\n")
		w.(http.Flusher).Flush()
		close(connected)
		<-r.Context().Done()
		close(closed)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream := startStream(ctx, streamRequest{protocol: "openai-chat", baseURL: server.URL, token: "test", definition: model{ID: "test"}})
	select {
	case <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	for range stream {
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("request survived cancellation")
	}
}

func TestProtocolContinuationsRemainScoped(t *testing.T) {
	for _, api := range []string{"anthropic", "openai-responses"} {
		r := streamRequest{protocol: api, definition: model{Provider: "one", ID: "model", ContextWindow: 4096}, messages: []modelMessage{{Role: RoleAssistant, Content: []part{{Type: PartReasoning, Continuation: &session.ModelContinuation{Provider: "one", Model: "model", API: api, Data: json.RawMessage(`{"type":"thinking","thinking":"private","signature":"signature"}`)}}}}}}
		makeBody := func() map[string]any {
			if api == "anthropic" {
				return r.anthropicBody()
			}
			return r.responsesBody()
		}
		body, _ := json.Marshal(makeBody())
		if !strings.Contains(string(body), "signature") {
			t.Fatal("continuation lost")
		}
		r.definition.Provider = "two"
		body, _ = json.Marshal(makeBody())
		if strings.Contains(string(body), "signature") {
			t.Fatal("continuation leaked across providers")
		}
	}
}

func TestAnthropicThinkingAndToolsRoundTrip(t *testing.T) {
	sse := []string{
		`{"type":"message_start","message":{"usage":{"input_tokens":2,"cache_read_input_tokens":3}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"plan"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"signed"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool-1","name":"read","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
		`{"type":"message_stop"}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range sse {
			var event struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal([]byte(line), &event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, line)
		}
	}))
	defer server.Close()
	r := streamRequest{protocol: "anthropic", baseURL: server.URL, token: "key", definition: model{Provider: "anthropic", ID: "model", ContextWindow: 4096}}
	var continuation *session.ModelContinuation
	called, finished := false, false
	for event := range startStream(t.Context(), r) {
		switch event.Type {
		case ChunkError:
			t.Fatal(event.Error)
		case ChunkContinuation:
			continuation = event.Continuation
		case ChunkToolCall:
			called = event.ToolCallID == "tool-1"
		case ChunkFinish:
			finished = event.Usage.InputTokens == 2 && event.Usage.CacheReadTokens == 3
		}
	}
	if continuation == nil || !called || !finished {
		t.Fatal("missing thinking/tool/usage")
	}
	r.messages = []modelMessage{{Role: RoleAssistant, Content: []part{{Type: PartReasoning, Continuation: continuation}, {Type: PartToolCall, ToolCallID: "tool-1", ToolName: "read", ToolInput: json.RawMessage(`{}`)}}}, {Role: RoleTool, Content: []part{{Type: PartToolResult, ToolCallID: "tool-1", ToolOutput: "ok"}}}}
	body, _ := json.Marshal(r.anthropicBody())
	if !strings.Contains(string(body), `"signature":"signed"`) || !strings.Contains(string(body), `"tool_use_id":"tool-1"`) {
		t.Fatalf("invalid replay: %s", body)
	}
}

func TestCodexRequestUsesSubscriptionShape(t *testing.T) {
	r := streamRequest{protocol: "openai-codex", definition: model{ID: "codex"}, options: map[string]any{"reasoning_effort": "high"}}
	body := r.responsesBody()
	if body["store"] != false || body["parallel_tool_calls"] != true || body["instructions"] == "" {
		t.Fatalf("bad subscription body: %+v", body)
	}
	if _, ok := body["max_output_tokens"]; ok {
		t.Fatal("Codex rejects explicit output cap")
	}
}

func TestResponsesKeepsMultimodalMessageAndToolOrder(t *testing.T) {
	r := streamRequest{definition: model{ID: "grok-4.7"}, messages: []modelMessage{
		{Role: RoleSystem, Content: []part{{Type: PartText, Text: "first"}, {Type: PartText, Text: "second"}}},
		{Role: RoleUser, Content: []part{{Type: PartText, Text: "look"}, {Type: PartImage, URL: "data:image/png;base64,AAAA"}, {Type: PartText, Text: "here"}}},
		{Role: RoleAssistant, Content: []part{{Type: PartText, Text: "checking"}, {Type: PartToolCall, ToolCallID: "call-1", ToolName: "read", ToolInput: json.RawMessage(`{}`)}}},
		{Role: RoleTool, Content: []part{{Type: PartToolResult, ToolCallID: "call-1", ToolOutput: "ok"}}},
	}}
	input := r.responsesBody()["input"].([]any)
	if len(input) != 5 {
		t.Fatalf("input items = %d: %+v", len(input), input)
	}
	if input[0].(map[string]any)["content"] != "first\nsecond" {
		t.Fatalf("system message = %+v", input[0])
	}
	user := input[1].(map[string]any)
	content := user["content"].([]any)
	if user["role"] != RoleUser || len(content) != 3 || content[1].(map[string]any)["type"] != "input_image" {
		t.Fatalf("user message split or reordered: %+v", user)
	}
	if input[2].(map[string]any)["role"] != RoleAssistant || input[3].(map[string]any)["type"] != "function_call" || input[4].(map[string]any)["type"] != "function_call_output" {
		t.Fatalf("assistant tool sequence changed: %+v", input)
	}
}

func TestResponsesFinalItemsWithoutDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[{"text":"plan"}],"encrypted_content":"sealed"}}`,
			`{"type":"response.output_item.done","output_index":1,"item":{"type":"message","content":[{"type":"output_text","text":"hello"}]}}`,
			`{"type":"response.output_item.done","output_index":2,"item":{"type":"reasoning","id":"rs_2","content":[{"type":"reasoning_text","text":" more"}]}}`,
			`{"type":"response.done","response":{"status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	defer server.Close()
	request := streamRequest{protocol: "openai-responses", baseURL: server.URL, token: "test", definition: model{Provider: "xai-oauth", ID: "grok-4.7"}}
	var reasoning, answer string
	var continuation *session.ModelContinuation
	finished := false
	for event := range startStream(t.Context(), request) {
		switch event.Type {
		case ChunkError:
			t.Fatal(event.Error)
		case ChunkReasoning:
			reasoning += event.Text
		case ChunkText:
			answer += event.Text
		case ChunkContinuation:
			continuation = event.Continuation
		case ChunkFinish:
			finished = true
		}
	}
	if reasoning != "plan more" || answer != "hello" || continuation == nil || !finished {
		t.Fatalf("lost final item: reasoning=%q answer=%q continuation=%+v finished=%v", reasoning, answer, continuation, finished)
	}
}

func TestOpenAICompatibleStringError(t *testing.T) {
	for _, protocol := range []string{"openai-chat", "openai-responses"} {
		t.Run(protocol, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"This account cannot access the requested model."}`)
			}))
			defer server.Close()
			request := streamRequest{protocol: protocol, baseURL: server.URL, token: "test", definition: model{Provider: "xai-oauth", ID: "grok-4.7"}}
			var failure error
			for event := range startStream(t.Context(), request) {
				if event.Type == ChunkError {
					failure = event.Error
				}
			}
			if failure == nil || !strings.Contains(failure.Error(), "HTTP 403: This account cannot access") {
				t.Fatalf("lost provider error: %v", failure)
			}
		})
	}
}

func TestChatPreservesStructuredReasoning(t *testing.T) {
	events := []string{
		`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","index":0,"text":"first "}]}}]}`,
		`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","index":0,"text":"second","signature":"signed"},{"type":"reasoning.encrypted","data":"opaque"}]},"finish_reason":"stop"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	defer server.Close()
	request := streamRequest{protocol: "openai-chat", baseURL: server.URL, token: "test", definition: model{Provider: "openrouter", ID: "test"}}
	var continuation *session.ModelContinuation
	for event := range startStream(t.Context(), request) {
		if event.Type == ChunkError {
			t.Fatal(event.Error)
		}
		if event.Type == ChunkContinuation {
			continuation = event.Continuation
		}
	}
	if continuation == nil {
		t.Fatal("missing reasoning details")
	}
	request.messages = []modelMessage{{Role: RoleAssistant, Content: []part{{Type: PartText, Text: "answer"}, {Type: PartReasoning, Continuation: continuation}}}}
	body, _ := json.Marshal(request.chatBody())
	if !strings.Contains(string(body), "first second") || !strings.Contains(string(body), `"data":"opaque"`) || !strings.Contains(string(body), `"content":"answer"`) {
		t.Fatalf("invalid replay: %s", body)
	}
	request.definition.Provider = "other"
	body, _ = json.Marshal(request.chatBody())
	if strings.Contains(string(body), "opaque") {
		t.Fatal("reasoning details crossed provider boundary")
	}
}

func TestChatToolSignatureRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"one","function":{"name":"read","arguments":"{}"},"extra_content":{"google":{"thought_signature":"signed"}}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
	}))
	defer server.Close()
	r := streamRequest{protocol: "openai-chat", baseURL: server.URL, token: "key", definition: model{Provider: "google", ID: "gemini"}}
	var block session.Block
	for chunk := range startStream(t.Context(), r) {
		if chunk.Type == ChunkError {
			t.Fatal(chunk.Error)
		}
		if chunk.Type == ChunkToolCall {
			block = session.Block{Kind: "tool-call", Tool: &session.ToolCall{ID: chunk.ToolCallID, Name: chunk.ToolName, Args: chunk.ToolInput}, Continuation: chunk.Continuation}
		}
	}
	converted, err := toProviderMessages([]session.Message{{Role: session.RoleAssistant, Blocks: []session.Block{block}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	r.messages = converted
	body, _ := json.Marshal(r.chatBody())
	if !strings.Contains(string(body), `"thought_signature":"signed"`) {
		t.Fatalf("signature lost: %s", body)
	}
	r.definition.Provider = "other"
	body, _ = json.Marshal(r.chatBody())
	if strings.Contains(string(body), "signed") {
		t.Fatal("tool signature crossed providers")
	}
}

// 防止名单越界、混合网关协议错配，以及手动地址被模型默认地址覆盖。
func TestPiProviderScopeAndRoutes(t *testing.T) {
	for _, id := range []string{"google", "google-vertex", "amazon-bedrock", "azure-openai-responses", "mistral", "radius", "typesafe", "tencent-tokenhub", "alibaba", "zhipuai"} {
		if _, exists := catalog.Providers[id]; exists {
			t.Fatalf("excluded provider present: %s", id)
		}
	}
	for key, definition := range catalog.Models {
		if !validProtocol(modelProtocol(definition, providerConfig{})) {
			t.Fatalf("invalid protocol: %s", key)
		}
	}
	for _, test := range []struct{ provider, protocol, want string }{
		{"opencode", "anthropic", "https://proxy.test"},
		{"fireworks", "anthropic", "https://proxy.test"},
		{"opencode", "openai-responses", "https://proxy.test/v1"},
		{"cloudflare-ai-gateway", "anthropic", "https://proxy.test/v1/anthropic"},
		{"cloudflare-ai-gateway", "openai-responses", "https://proxy.test/v1/openai"},
		{"cloudflare-ai-gateway", "openai-chat", "https://proxy.test/v1/compat"},
	} {
		definition := model{Provider: test.provider, Protocol: test.protocol}
		if got := modelURL(definition, providerConfig{BaseURL: "https://proxy.test/v1"}); got != test.want {
			t.Fatalf("%s: %s != %s", test.provider, got, test.want)
		}
	}
	if err := validateProvider(SaveProviderInput{ID: "cloudflare-workers-ai", Protocol: "openai-chat"}); err == nil {
		t.Fatal("unresolved account ID accepted")
	}
}
