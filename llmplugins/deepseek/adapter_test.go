package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"harness/agents"
	"harness/chat"
	"harness/core"
	"harness/llm"
	"harness/localenv"
	"harness/loop"
	"harness/persistence/presetjson"
	"harness/persistence/projectjson"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

type testJournalPlugin struct {
	journal session.Journal
}

func (testJournalPlugin) Name() string {
	return "test-journal"
}

func (p testJournalPlugin) Start(app *core.App) error {
	app.RegisterService("journal", p.journal)
	return nil
}

func TestAdapterSendsThinkingAndReadsStream(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Fatalf("请求地址不对：%s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("SDK 没有使用配置里的 API key")
		}
		data, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = json.Unmarshal(data, &body)
		if err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: {\"id\":\"r1\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"先想\",\"content\":\"我来\",\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"write_file\",\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\n")
		_, _ = io.WriteString(writer, "data: {\"id\":\"r1\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"再做\",\"content\":\"这就写\",\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"a.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		_, _ = io.WriteString(writer, "data: {\"id\":\"r1\",\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n")
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}))
	defer server.Close()

	adapter := newAdapter("test-key", server.URL)
	var visible strings.Builder
	reply, err := adapter.Stream(context.Background(), llm.Request{
		Provider: "deepseek",
		Model:    "deepseek-reasoner",
		Thinking: "high",
		Messages: []chat.Message{
			{Role: "user", Text: "写文件"},
			{
				Role:     "assistant",
				Text:     "上一轮",
				Thinking: "上一轮思考",
				Calls: []chat.ToolCall{{
					ID:       "old-call",
					Name:     "write_file",
					Argument: json.RawMessage(`{"path":"old.txt"}`),
				}},
			},
			{Role: "tool", Result: &chat.ToolResult{CallID: "old-call", Output: "写好了", Status: "success"}},
		},
		Tools: []chat.ToolSchema{{
			Name:        "write_file",
			Description: "写文件",
			Parameters:  []byte(`{"type":"object"}`),
		}},
	}, func(delta chat.Delta) {
		visible.WriteString(delta.Text)
	})
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	if visible.String() != "我来这就写" || reply.Text != visible.String() {
		t.Fatalf("可见文字不对：%q / %q", visible.String(), reply.Text)
	}
	if reply.Thinking != "先想再做" {
		t.Fatalf("隐藏思考不对：%q", reply.Thinking)
	}
	if len(reply.Calls) != 1 || reply.Calls[0].ID != "call-1" || reply.Calls[0].Name != "write_file" || string(reply.Calls[0].Argument) != `{"path":"a.txt"}` {
		t.Fatalf("工具调用没有合并好：%+v", reply.Calls)
	}
	if reply.Usage.InputTokens != 11 || reply.Usage.OutputTokens != 7 || reply.StopReason != "tool_calls" {
		t.Fatalf("用量或结束原因不对：%+v", reply)
	}
	if body["reasoning_effort"] != "high" {
		t.Fatalf("请求没有带 reasoning_effort：%v", body)
	}
	thinking := body["thinking"].(map[string]any)
	if thinking["type"] != "enabled" {
		t.Fatalf("high 必须明确开启 thinking：%v", thinking)
	}
	messages := body["messages"].([]any)
	assistant := messages[1].(map[string]any)
	if assistant["reasoning_content"] != "上一轮思考" {
		t.Fatalf("历史没有带 reasoning_content：%v", assistant)
	}
	if len(assistant["tool_calls"].([]any)) != 1 {
		t.Fatalf("历史工具调用没有和助手消息一起发送：%v", assistant)
	}
}

func TestAdapterRejectsUnknownThinking(t *testing.T) {
	adapter := newAdapter("test-key", "http://127.0.0.1")
	_, err := adapter.Stream(context.Background(), llm.Request{Thinking: "medium"}, nil)
	if err == nil {
		t.Fatal("未知 thinking 应在发请求前拒绝")
	}
	typed, ok := err.(*llm.Error)
	if !ok || typed.Kind != llm.ErrBadRequest {
		t.Fatalf("错误类别不对：%v", err)
	}
}

func TestAdapterTurnsThinkingOff(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = json.Unmarshal(data, &body)
		if err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}))
	defer server.Close()

	adapter := newAdapter("test-key", server.URL)
	_, err := adapter.Stream(context.Background(), llm.Request{Model: "m", Thinking: "off"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	thinking := body["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("off 必须明确关闭 thinking：%v", thinking)
	}
	if _, exists := body["reasoning_effort"]; exists {
		t.Fatalf("关思考时不能发送 reasoning_effort：%v", body)
	}
}

func TestAdapterClassifiesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, `{"error":{"message":"慢一点","type":"rate_limit_error"}}`)
	}))
	defer server.Close()

	adapter := newAdapter("test-key", server.URL)
	_, err := adapter.Stream(context.Background(), llm.Request{Thinking: "off"}, nil)
	if err == nil {
		t.Fatal("429 应报错")
	}
	typed, ok := err.(*llm.Error)
	if !ok || typed.Kind != llm.ErrRateLimit {
		t.Fatalf("429 应归类为限流：%v", err)
	}
}

func TestPluginRegistersDeepSeek(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"error":{"message":"坏请求","type":"invalid_request_error"}}`)
	}))
	defer server.Close()
	app := core.New()
	t.Cleanup(app.Close)
	err := app.Install(llm.Plugin{}, Plugin{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("装插件失败：%v", err)
	}
	service, err := llm.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Complete(context.Background(), llm.Request{Provider: "deepseek", Model: "m", Thinking: "off"})
	if err == nil {
		t.Fatal("测试服务的坏请求应返回错误")
	}
	if !called {
		t.Fatal("指定 deepseek 后应路由到 DeepSeek 适配器")
	}
}

func TestLoopSendsThinkingWithAssistantToolCallOnSecondRequest(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		var body map[string]any
		err = json.Unmarshal(data, &body)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, body)
		index := len(requests)
		mu.Unlock()

		writer.Header().Set("Content-Type", "text/event-stream")
		if index == 1 {
			_, _ = io.WriteString(writer, "data: {\"id\":\"r1\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"先检查参数\",\"content\":\"我来回显\",\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"echo\",\"arguments\":\"{\\\"text\\\":\\\"你好\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		} else {
			_, _ = io.WriteString(writer, "data: {\"id\":\"r2\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"已完成\"},\"finish_reason\":\"stop\"}]}\n\n")
		}
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}))
	defer server.Close()

	app := core.New()
	t.Cleanup(app.Close)
	err := app.Install(
		projectjson.Plugin{Root: t.TempDir()},
		presetjson.Plugin{Root: t.TempDir()},
		testJournalPlugin{journal: session.NewMemoryJournal()},
		session.Plugin{},
		llm.Plugin{},
		Plugin{APIKey: "test-key", BaseURL: server.URL},
		tools.Plugin{},
		localenv.Plugin{},
		projects.Plugin{},
		presets.Plugin{},
		agents.Plugin{},
		loop.Plugin{},
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tools.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	err = registry.Register(tools.Tool{
		Schema: chat.ToolSchema{Name: "echo", Parameters: []byte(`{"type":"object"}`)},
		Execute: func(ctx context.Context, scope *core.App, argument json.RawMessage) (string, error) {
			return string(argument), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	projectService, err := projects.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectService.Create("测试项目", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	presetService, err := presets.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	err = presetService.Create(presets.Preset{
		ID:       "小红",
		Provider: "deepseek",
		Model:    "deepseek-reasoner",
		Thinking: "high",
		Tools:    []string{"echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := agents.Get(app)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := service.StartSession(agents.StartInput{ProjectID: project.ID, PresetID: "小红", Title: "会话一"})
	if err != nil {
		t.Fatal(err)
	}
	err = conversation.SubmitFollowup("帮我回显你好")
	if err != nil {
		t.Fatal(err)
	}
	conversation.WaitIdle()

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("工具调用后应请求两次模型：%d", len(requests))
	}
	messages := requests[1]["messages"].([]any)
	found := false
	for _, item := range messages {
		message := item.(map[string]any)
		if message["role"] != "assistant" || message["reasoning_content"] != "先检查参数" {
			continue
		}
		calls, exists := message["tool_calls"].([]any)
		if exists && len(calls) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("第二次请求没有把 reasoning_content 和工具调用合成助手消息：%v", messages)
	}
}
