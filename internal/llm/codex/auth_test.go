package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"harness/internal/llm/oauth"
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
		verifier: "verifier", server: &http.Server{}, view: oauth.View{State: "waiting"}}
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

// 令牌刷新必须先落盘，之后才能发起模型请求。
func TestCodexRefreshAndPersistence(t *testing.T) {
	jwt := testCodexJWT("account-1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil || r.Form.Get("refresh_token") != "old-refresh" {
			t.Error("wrong refresh request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"new-refresh","expires_in":3600}`, jwt)
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
	defer auth.Close()
	auth.credential = &credential{Access: "old", Refresh: "old-refresh", Expires: time.Now().Add(-time.Minute), AccountID: "old-account"}
	auth.tokenURL = server.URL
	value, err := auth.RequestAuth(t.Context())
	if err != nil || value.Token != jwt || value.AccountID != "account-1" {
		t.Fatalf("credentials: %+v, %v", value, err)
	}
	restored, err := New(files)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.credential.Refresh != "new-refresh" {
		t.Fatal("refresh not saved")
	}
	_, err = auth.Logout()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RequestAuth(t.Context()); err == nil {
		t.Fatal("logged out token still usable")
	}
}
