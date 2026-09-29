package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zendev-sh/goai/provider"

	"harness/internal/persist"
)

func testCodexJWT(account string) string {
	payload, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": account}})
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

// 错误 state 不能交换授权码，更不能覆盖已保存的订阅凭据。
func TestCodexCallbackRejectsWrongState(t *testing.T) {
	token := testCodexJWT("account-1")
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		requests <- request.Form.Get("code")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"refresh","expires_in":3600}`, token)
	}))
	defer server.Close()
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	auth, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	auth.tokenURL = server.URL
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	task := &authTask{state: "expected", ctx: ctx, cancel: cancel,
		verifier: "verifier", server: &http.Server{}, view: View{State: "waiting"}}
	auth.task = task
	response := httptest.NewRecorder()
	auth.callback(task, response, httptest.NewRequest(http.MethodGet, "/auth/callback?state=wrong&code=secret", nil))
	if response.Code != http.StatusBadRequest || auth.Status().State != "waiting" {
		t.Fatalf("callback=%d, state=%s", response.Code, auth.Status().State)
	}
	select {
	case code := <-requests:
		t.Fatalf("wrong state exchanged code %q", code)
	default:
	}
	accepted := httptest.NewRecorder()
	auth.callback(task, accepted, httptest.NewRequest(http.MethodGet, "/auth/callback?state=expected&code=valid", nil))
	if accepted.Code != http.StatusOK {
		t.Fatalf("valid callback: %d", accepted.Code)
	}
	select {
	case code := <-requests:
		if code != "valid" {
			t.Fatalf("exchanged %q", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("token exchange did not start")
	}
	deadline := time.After(2 * time.Second)
	for auth.Status().State != "complete" {
		select {
		case <-deadline:
			t.Fatalf("login: %+v", auth.Status())
		case <-time.After(5 * time.Millisecond):
		}
	}
	if !auth.Authenticated() {
		t.Fatal("credential was not saved")
	}
}

// 刷新、持久化和订阅模型流必须共享同一个账号；请求不带 API Key。
func TestCodexRefreshAndStream(t *testing.T) {
	jwt := testCodexJWT("account-1")
	type requestView struct {
		path, auth, account, beta string
		body                      map[string]any
	}
	requests := make(chan requestView, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/token" {
			if err := request.ParseForm(); err != nil || request.Form.Get("refresh_token") != "old-refresh" {
				t.Errorf("refresh request: %v, %v", request.Form, err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"new-refresh","expires_in":3600}`, jwt)
			return
		}
		body, _ := io.ReadAll(request.Body)
		var decoded map[string]any
		_ = json.Unmarshal(body, &decoded)
		requests <- requestView{path: request.URL.Path, auth: request.Header.Get("Authorization"),
			account: request.Header.Get("chatgpt-account-id"), beta: request.Header.Get("OpenAI-Beta"), body: decoded}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.output_item.added\n"+
			`data: {"output_index":0,"item":{"type":"function_call","call_id":"tc1","name":"read_file"}}`+"\n\n"+
			"event: response.function_call_arguments.delta\n"+`data: {"output_index":0,"delta":"{}"}`+"\n\n"+
			"event: response.function_call_arguments.done\n"+`data: {"output_index":0,"arguments":"{}"}`+"\n\n"+
			"event: response.output_item.done\n"+`data: {"output_index":0}`+"\n\n"+
			"event: response.completed\n"+`data: {"response":{"usage":{"input_tokens":2,"output_tokens":3}}}`+"\n\n")
	}))
	defer server.Close()
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	auth, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	auth.credential = &credential{Access: testCodexJWT("old-account"), Refresh: "old-refresh",
		Expires: time.Now().Add(-time.Minute), AccountID: "old-account"}
	auth.tokenURL = server.URL + "/token"
	auth.baseURL = server.URL + "/codex"
	model, err := auth.Model(context.Background(), "gpt-6-sol")
	if err != nil {
		t.Fatal(err)
	}
	response, err := model.DoStream(context.Background(), provider.GenerateParams{
		System:          "Be helpful",
		Messages:        []provider.Message{{Role: provider.RoleUser, Content: []provider.Part{{Type: provider.PartText, Text: "read"}}}},
		Tools:           []provider.ToolDefinition{{Name: "read_file", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		ProviderOptions: Options(map[string]any{"reasoning_effort": "low"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	call := false
	for chunk := range response.Stream {
		if chunk.Type == provider.ChunkError {
			t.Fatalf("stream: %v", chunk.Error)
		}
		if chunk.Type == provider.ChunkToolCall && chunk.ToolName == "read_file" {
			call = true
		}
	}
	if !call {
		t.Fatal("missing tool call")
	}
	request := <-requests
	if request.path != "/codex/responses" || request.auth != "Bearer "+jwt ||
		request.account != "account-1" || request.beta != "responses=experimental" {
		t.Fatalf("request headers/path: %+v", request)
	}
	if request.body["store"] != false || request.body["parallel_tool_calls"] != true || request.body["instructions"] != "Be helpful" {
		t.Fatalf("request body: %+v", request.body)
	}
	if reasoning, ok := request.body["reasoning"].(map[string]any); !ok || reasoning["effort"] != "low" {
		t.Fatalf("reasoning: %+v", request.body["reasoning"])
	}
	if includes, ok := request.body["include"].([]any); !ok || len(includes) != 1 || includes[0] != "reasoning.encrypted_content" {
		t.Fatalf("include: %+v", request.body["include"])
	}
	restored, err := New(files)
	if err != nil || !restored.Authenticated() {
		t.Fatalf("stored token: %v", err)
	}
	view, err := auth.Logout()
	if err != nil || view.Authenticated {
		t.Fatalf("logout: %+v, %v", view, err)
	}
	if _, err := auth.token(t.Context()); err == nil || !strings.Contains(err.Error(), "登录") {
		t.Fatalf("logout token: %v", err)
	}
}
