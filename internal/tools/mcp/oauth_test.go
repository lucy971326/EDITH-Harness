package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
	"harness/internal/approvals"
	"harness/internal/llm"
	"harness/internal/permissions"
	"harness/internal/persist"
	"harness/internal/session"
	"harness/internal/tools"
)

// 文件存储必须跨进程可读，并在退出登录时清除；Windows 文件不能含明文令牌。
func TestOAuth_fileStoreSurvivesRestartAndLogout(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authFiles, err := files.Scope("mcp", "oauth")
	if err != nil {
		t.Fatal(err)
	}
	first := newOAuthStore()
	first.files = authFiles
	record := oauthRecord{Resource: "https://example.com/mcp", ClientID: "client", Token: oauth2.Token{AccessToken: "access-secret", RefreshToken: "refresh-secret"}}
	persisted, _, err := first.issue("demo", 0, record)
	if err != nil || !persisted {
		t.Fatalf("save credential: persisted=%v, err=%v", persisted, err)
	}
	raw, err := authFiles.Read("demo.cred")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && bytes.Contains(raw, []byte("access-secret")) {
		t.Fatal("Windows credential file contains a plain token")
	}
	if runtime.GOOS != "windows" {
		path, err := authFiles.Path("demo.cred")
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("credential file permissions: %v, %v", info, err)
		}
	}
	restarted := newOAuthStore()
	restarted.files = authFiles
	got, found, err := restarted.read("demo")
	if err != nil || !found || got.Token.AccessToken != record.Token.AccessToken || got.Token.RefreshToken != record.Token.RefreshToken {
		t.Fatalf("restart credential: found=%v, err=%v", found, err)
	}
	if err := restarted.delete("demo"); err != nil {
		t.Fatal(err)
	}
	last := newOAuthStore()
	last.files = authFiles
	if _, found, err := last.read("demo"); err != nil || found {
		t.Fatalf("logout credential: found=%v, err=%v", found, err)
	}
}

// 保存失败不能报告授权完成，也不能推进该连接的凭据代数。
func TestOAuth_fileWriteFailureDoesNotIssue(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := files.Write("blocked", []byte("file")); err != nil {
		t.Fatal(err)
	}
	blocked, err := files.Scope("blocked")
	if err != nil {
		t.Fatal(err)
	}
	store := newOAuthStore()
	store.files = blocked
	_, epoch, err := store.issue("demo", 0, oauthRecord{Token: oauth2.Token{AccessToken: "secret"}})
	if err == nil || epoch != 0 || store.credentialEpoch("demo") != 0 || len(store.memory) != 0 {
		t.Fatalf("failed issue: epoch=%d, err=%v", epoch, err)
	}
}

// 真 HTTP 流程守住普通 Run 不弹浏览器、显式授权、取消和带 resource 的刷新。
func TestOAuth_explicitLoginAndRefresh(t *testing.T) {
	mcpServer := sdk.NewServer(&sdk.Implementation{Name: "oauth-test", Version: "v1"}, nil)
	mcpServer.AddTool(&sdk.Tool{Name: "ping", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		})
	mcpHandler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return mcpServer },
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	var remote *httptest.Server
	var authorizations, refreshes, untrustedMetadata atomic.Int32
	var wrongIssuer atomic.Bool
	var switchedTokenEndpoint atomic.Bool
	var refreshResource atomic.Value
	remote = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mcp":
			if r.Header.Get("Authorization") != "Bearer access" && r.Header.Get("Authorization") != "Bearer renewed" {
				w.Header().Set("WWW-Authenticate", `Basic resource_metadata="`+remote.URL+`/untrusted", Bearer resource_metadata="`+remote.URL+`/.well-known/oauth-protected-resource/mcp"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			mcpHandler.ServeHTTP(w, r)
		case "/untrusted":
			untrustedMetadata.Add(1)
			http.NotFound(w, r)
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": remote.URL + "/mcp", "authorization_servers": []string{remote.URL}})
		case "/.well-known/oauth-authorization-server":
			tokenEndpoint := remote.URL + "/token"
			if switchedTokenEndpoint.Load() {
				tokenEndpoint = remote.URL + "/token-new"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": remote.URL,
				"authorization_endpoint": remote.URL + "/authorize", "token_endpoint": tokenEndpoint,
				"registration_endpoint": remote.URL + "/register", "response_types_supported": []string{"code"},
				"code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}})
		case "/register":
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "harness-test", "token_endpoint_auth_method": "none"})
		case "/authorize":
			authorizations.Add(1)
			redirect, err := url.Parse(r.URL.Query().Get("redirect_uri"))
			if err != nil {
				http.Error(w, "bad redirect", 400)
				return
			}
			query := redirect.Query()
			query.Set("code", "code")
			query.Set("state", r.URL.Query().Get("state"))
			issuer := remote.URL
			if wrongIssuer.Load() {
				issuer = "https://other.example"
			}
			query.Set("iss", issuer)
			redirect.RawQuery = query.Encode()
			http.Redirect(w, r, redirect.String(), http.StatusFound)
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("resource") != remote.URL+"/mcp" {
				http.Error(w, "missing resource", 400)
				return
			}
			if r.Form.Get("grant_type") == "refresh_token" {
				refreshes.Add(1)
				refreshResource.Store(r.Form.Get("resource"))
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "renewed", "token_type": "Bearer", "refresh_token": "rotated", "expires_in": 3600})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "token_type": "Bearer", "refresh_token": "initial", "expires_in": 3600})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	provider, err := newProvider(t.Context(), configFile{MCPServers: map[string]serverConfig{
		"demo": {Type: "http", URL: remote.URL + "/mcp"},
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider.authStore.memoryOnly = true
	defer provider.Close()
	provider.invalidate("")
	snapshot, err := provider.Snapshot(tools.WithAccess(t.Context(), tools.Access{Mode: permissions.FullAccess}), "")
	if err != nil || len(snapshot.Definitions) != 0 || authorizations.Load() != 0 {
		t.Fatalf("unauthenticated Run: %#v, %v", snapshot, err)
	}
	view, err := provider.ReadSettings("")
	if err != nil || view.Global[0].Status != "auth-required" {
		t.Fatalf("auth status: %#v, %v", view, err)
	}
	cancelled, err := provider.StartOAuth("", "global", "demo")
	if err != nil || cancelled.URL == "" {
		t.Fatalf("start login: %#v, %v", cancelled, err)
	}
	recovered, err := provider.StartOAuth("", "global", "demo")
	if err != nil || recovered.ID != cancelled.ID || recovered.URL != cancelled.URL {
		t.Fatalf("restore waiting task: %#v, %v", recovered, err)
	}
	cancelled, err = provider.CancelOAuth(cancelled.ID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatalf("cancel login: %#v, %v", cancelled, err)
	}
	started, err := provider.StartOAuth("", "global", "demo")
	if err != nil || started.URL == "" {
		t.Fatalf("restart login: %#v, %v", started, err)
	}
	authorizeURL, err := url.Parse(started.URL)
	if err != nil {
		t.Fatal(err)
	}
	callbackURL := authorizeURL.Query().Get("redirect_uri")
	wrong, err := http.Get(callbackURL + "?code=code&state=wrong")
	if err != nil {
		t.Fatal(err)
	}
	wrong.Body.Close()
	if wrong.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong OAuth state = %d", wrong.StatusCode)
	}
	stillWaiting, err := provider.OAuthStatus(started.ID)
	if err != nil || stillWaiting.State != "waiting" {
		t.Fatalf("bad callback finished login: %#v, %v", stillWaiting, err)
	}
	response, err := http.Get(started.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	deadline := time.After(5 * time.Second)
	for {
		status, err := provider.OAuthStatus(started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "complete" {
			break
		}
		if status.State == "failed" {
			t.Fatalf("login failed: %#v", status)
		}
		select {
		case <-deadline:
			t.Fatal("login did not finish")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if authorizations.Load() != 1 || untrustedMetadata.Load() != 0 {
		t.Fatalf("browser auth count = %d, untrusted discovery = %d", authorizations.Load(), untrustedMetadata.Load())
	}
	key := oauthKey("", false, serverSpec{Name: "demo", URL: remote.URL + "/mcp"})
	record, found, err := provider.authStore.read(key)
	if err != nil || !found {
		t.Fatalf("credential missing: %v", err)
	}
	wrongIssuer.Store(true)
	rejected, err := provider.StartOAuth("", "global", "demo")
	if err != nil || rejected.URL == "" {
		t.Fatalf("start issuer test: %#v, %v", rejected, err)
	}
	response, err = http.Get(rejected.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	for deadline := time.After(5 * time.Second); ; {
		status, err := provider.OAuthStatus(rejected.ID)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "failed" {
			break
		}
		if status.State == "complete" {
			t.Fatal("accepted wrong issuer")
		}
		select {
		case <-deadline:
			t.Fatal("wrong issuer did not finish")
		case <-time.After(20 * time.Millisecond):
		}
	}
	wrongIssuer.Store(false)
	unchanged, _, _ := provider.authStore.read(key)
	if unchanged.Token.AccessToken != record.Token.AccessToken {
		t.Fatal("wrong issuer replaced credential")
	}
	record.Token.Expiry = time.Now().Add(-time.Minute)
	if _, err := provider.authStore.write(key, record); err != nil {
		t.Fatal(err)
	}
	provider.invalidate("")
	snapshot, err = provider.Snapshot(tools.WithAccess(t.Context(), tools.Access{Mode: permissions.FullAccess}), "")
	if err != nil || len(snapshot.Definitions) != 1 || refreshes.Load() == 0 || refreshResource.Load() != remote.URL+"/mcp" {
		t.Fatalf("refresh snapshot: %#v, %v, refresh=%d", snapshot, err, refreshes.Load())
	}
	record, _, _ = provider.authStore.read(key)
	if record.Token.RefreshToken != "rotated" {
		t.Fatalf("refresh token not rotated")
	}
	switchedTokenEndpoint.Store(true)
	provider.invalidate("")
	refreshCount := refreshes.Load()
	snapshot, err = provider.Snapshot(tools.WithAccess(t.Context(), tools.Access{Mode: permissions.FullAccess}), "")
	if err != nil || len(snapshot.Definitions) != 0 || refreshes.Load() != refreshCount {
		t.Fatalf("changed token endpoint reused credential: %#v, %v", snapshot, err)
	}
	if _, err := provider.LogoutOAuth("", "global", "demo"); err != nil {
		t.Fatal(err)
	}
	handler := &savedTokenSource{store: provider.authStore, key: key, version: provider.authStore.version(key) - 1, record: unchanged}
	if _, err := handler.Token(); err == nil {
		t.Fatal("old connection accepted logout")
	}
	if _, found, _ := provider.authStore.read(key); found {
		t.Fatal("logout kept credentials")
	}
}

func TestOAuth_discoveryRequiresS256(t *testing.T) {
	var remote *httptest.Server
	remote = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+remote.URL+`/.well-known/oauth-protected-resource/mcp"`)
			w.WriteHeader(401)
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": remote.URL + "/mcp", "authorization_servers": []string{remote.URL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": remote.URL, "authorization_endpoint": remote.URL + "/authorize", "token_endpoint": remote.URL + "/token", "response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"plain"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	provider, err := newProvider(t.Context(), configFile{MCPServers: map[string]serverConfig{"demo": {Type: "http", URL: remote.URL + "/mcp"}}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider.authStore.memoryOnly = true
	defer provider.Close()
	task, err := provider.StartOAuth("", "global", "demo")
	if err != nil || task.State != "failed" || !strings.Contains(task.Message, "S256") {
		t.Fatalf("S256 rejection = %#v, %v", task, err)
	}
}

func TestOAuth_registrationUnavailableReturnsActionableTask(t *testing.T) {
	var remote *httptest.Server
	remote = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+remote.URL+`/.well-known/oauth-protected-resource/mcp"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": remote.URL + "/mcp", "authorization_servers": []string{remote.URL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": remote.URL, "authorization_endpoint": remote.URL + "/authorize",
				"token_endpoint": remote.URL + "/token", "response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	provider, err := newProvider(t.Context(), configFile{MCPServers: map[string]serverConfig{"demo": {Type: "http", URL: remote.URL + "/mcp"}}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	task, err := provider.StartOAuth("", "global", "demo")
	if err != nil || task.State != "failed" || task.Reason != "client-registration-required" || task.URL != "" {
		t.Fatalf("registration failure = %#v, %v", task, err)
	}
}

func TestOAuth_oldRunCannotOverwriteNewLogin(t *testing.T) {
	store := newOAuthStore()
	store.memoryOnly = true
	first := oauthRecord{Resource: "https://example.com/mcp", Token: oauth2.Token{AccessToken: "first"}}
	_, firstEpoch, err := store.issue("key", 0, first)
	if err != nil {
		t.Fatal(err)
	}
	second := oauthRecord{Resource: first.Resource, Token: oauth2.Token{AccessToken: "second"}}
	_, _, err = store.issue("key", 0, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.refresh("key", 0, firstEpoch, first); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.read("key")
	if err != nil || !found || got.Token.AccessToken != "second" {
		t.Fatalf("old refresh replaced login: %#v, %v", got, err)
	}
	if err := store.delete("key"); err != nil {
		t.Fatal(err)
	}
	if err := store.refresh("key", 0, firstEpoch, first); err == nil {
		t.Fatal("logout accepted old refresh")
	}
}

func TestOAuth_configRetirementKeepsOldRunButNotSavedCredential(t *testing.T) {
	store := newOAuthStore()
	store.memoryOnly = true
	record := oauthRecord{Resource: "https://example.com/mcp", Token: oauth2.Token{AccessToken: "old", Expiry: time.Now().Add(time.Hour)}}
	_, epoch, err := store.issue("key", 0, record)
	if err != nil {
		t.Fatal(err)
	}
	oldRun := &savedTokenSource{store: store, key: "key", version: 0, epoch: epoch, record: record}
	if err := store.retire("key"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.read("key"); found {
		t.Fatal("retired credential remained saved")
	}
	token, err := oldRun.Token()
	if err != nil || token.AccessToken != "old" {
		t.Fatalf("old Run lost fixed token: %#v, %v", token, err)
	}
}

func TestOAuth_concurrentRefreshUsesRotatedTokenOnce(t *testing.T) {
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("refresh_token") != "first" || r.Form.Get("resource") != "https://example.com/mcp" {
			http.Error(w, "stale refresh token", http.StatusBadRequest)
			return
		}
		refreshes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "renewed", "refresh_token": "second", "token_type": "Bearer", "expires_in": 3600})
	}))
	defer server.Close()
	store := newOAuthStore()
	store.memoryOnly = true
	record := oauthRecord{Resource: "https://example.com/mcp", ClientID: "client", TokenURL: server.URL,
		Token: oauth2.Token{AccessToken: "expired", RefreshToken: "first", Expiry: time.Now().Add(-time.Minute)}}
	_, epoch, err := store.issue("key", 0, record)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			source := &savedTokenSource{store: store, key: "key", epoch: epoch, record: record, client: server.Client()}
			token, err := source.Token()
			if err == nil && token.AccessToken != "renewed" {
				err = fmt.Errorf("token = %q", token.AccessToken)
			}
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if refreshes.Load() != 1 {
		t.Fatalf("refresh count = %d", refreshes.Load())
	}
}

func TestOAuth_disabledServerCanLogoutAndConfigChangeRevokesOldCredential(t *testing.T) {
	oldURL := "http://127.0.0.1:1/mcp"
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider, err := New(files, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	provider.authStore.memoryOnly = true
	view, err := provider.ReadSettings("")
	if err != nil {
		t.Fatal(err)
	}
	protocol := "http"
	view, err = provider.Save(t.Context(), SaveInput{Name: "demo", Revision: view.Revision, Create: true, Type: &protocol, URL: &oldURL})
	if err != nil {
		t.Fatal(err)
	}
	oldKey := oauthKey("", false, serverSpec{Name: "demo", URL: oldURL})
	record := oauthRecord{Resource: oldURL, Token: oauth2.Token{AccessToken: "secret"}}
	if _, err := provider.authStore.write(oldKey, record); err != nil {
		t.Fatal(err)
	}
	view, err = provider.ReadSettings("")
	if err != nil || !view.Global[0].HasOAuthCredentials {
		t.Fatalf("credential view: %#v, %v", view, err)
	}
	disabled := false
	view, err = provider.Save(t.Context(), SaveInput{Name: "demo", Revision: view.Revision, Enabled: &disabled})
	if err != nil || !view.Global[0].HasOAuthCredentials {
		t.Fatalf("disabled credential view: %#v, %v", view, err)
	}
	if _, err := provider.LogoutOAuth("", "global", "demo"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := provider.authStore.read(oldKey); found {
		t.Fatal("disabled server kept credential")
	}
	if _, err := provider.authStore.write(oldKey, record); err != nil {
		t.Fatal(err)
	}
	newURL := "http://127.0.0.1:2/mcp"
	view, err = provider.ReadSettings("")
	if err != nil {
		t.Fatal(err)
	}
	view, err = provider.Save(t.Context(), SaveInput{Name: "demo", Revision: view.Revision, URL: &newURL})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, _ := provider.authStore.read(oldKey); found {
		t.Fatal("changed server kept old credential")
	}
	newKey := oauthKey("", false, serverSpec{Name: "demo", URL: newURL})
	if _, err := provider.authStore.write(newKey, record); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Delete(t.Context(), "demo", view.Revision); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := provider.authStore.read(newKey); found {
		t.Fatal("deleted server kept credential")
	}
}

func TestOAuth_localhostTriesBothAddressFamilies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	u.Host = "localhost:" + u.Port()
	response, err := oauthHTTPClient(u.String()).Get(u.String())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("localhost status = %d", response.StatusCode)
	}
}

func TestOAuth_projectTrustSameNameAndConfigChange(t *testing.T) {
	var remote *httptest.Server
	remote = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/global" || r.URL.Path == "/project":
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+remote.URL+`/.well-known/oauth-protected-resource`+r.URL.Path+`"`)
			w.WriteHeader(http.StatusUnauthorized)
		case strings.HasPrefix(r.URL.Path, "/.well-known/oauth-protected-resource/"):
			resource := remote.URL + strings.TrimPrefix(r.URL.Path, "/.well-known/oauth-protected-resource")
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "authorization_servers": []string{remote.URL}})
		case r.URL.Path == "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": remote.URL, "authorization_endpoint": remote.URL + "/authorize",
				"token_endpoint": remote.URL + "/token", "registration_endpoint": remote.URL + "/register",
				"response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"},
				"token_endpoint_auth_methods_supported": []string{"none"}})
		case r.URL.Path == "/register":
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "test-client", "token_endpoint_auth_method": "none"})
		case r.URL.Path == "/authorize":
			redirect, err := url.Parse(r.URL.Query().Get("redirect_uri"))
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			query := redirect.Query()
			query.Set("code", "code")
			query.Set("state", r.URL.Query().Get("state"))
			query.Set("iss", remote.URL)
			redirect.RawQuery = query.Encode()
			http.Redirect(w, r, redirect.String(), http.StatusFound)
		case r.URL.Path == "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "token_type": "Bearer", "expires_in": 3600})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	workspace := t.TempDir()
	projectURL := remote.URL + "/project"
	project := configFile{MCPServers: map[string]serverConfig{"shared": {Type: "http", URL: projectURL}}}
	projectBody, _ := json.Marshal(project)
	projectPath := filepath.Join(workspace, ".mcp.json")
	if err := os.WriteFile(projectPath, projectBody, 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	global := configFile{MCPServers: map[string]serverConfig{"shared": {Type: "http", URL: remote.URL + "/global"}}}
	globalBody, _ := json.Marshal(global)
	if err := files.Write("mcp.json", globalBody); err != nil {
		t.Fatal(err)
	}
	if err := files.Write("config.yaml", []byte("jev: {}\n")); err != nil {
		t.Fatal(err)
	}
	approval, err := approvals.Open(files, &llm.Client{}, &session.Store{})
	if err != nil {
		t.Fatal(err)
	}
	defer approval.Close()
	provider, err := New(files, approval)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	provider.authStore.memoryOnly = true
	view, err := provider.ReadSettings(workspace)
	if err != nil || len(view.Global) != 1 || !view.Global[0].Overridden || len(view.Project) != 1 {
		t.Fatalf("scope view = %#v, %v", view, err)
	}
	globalTask, err := provider.StartOAuth("", "global", "shared")
	if err != nil || globalTask.URL == "" {
		t.Fatalf("global login = %#v, %v", globalTask, err)
	}
	if _, err := provider.CancelOAuth(globalTask.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.StartOAuth(workspace, "project", "shared"); !errors.Is(err, ErrOAuthUntrusted) {
		t.Fatalf("untrusted project login = %v", err)
	}
	if err := provider.TrustProjectOAuth(workspace, view.ProjectVersion); err != nil {
		t.Fatal(err)
	}
	projectTask, err := provider.StartOAuth(workspace, "project", "shared")
	if err != nil || projectTask.URL == "" {
		t.Fatalf("project login = %#v, %v", projectTask, err)
	}
	project.MCPServers["shared"] = serverConfig{Type: "http", URL: projectURL, ExcludeTools: []string{"ping"}}
	projectBody, _ = json.Marshal(project)
	if err := os.WriteFile(projectPath, projectBody, 0o600); err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(projectTask.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	for deadline := time.After(5 * time.Second); ; {
		status, err := provider.OAuthStatus(projectTask.ID)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "failed" {
			break
		}
		if status.State == "complete" {
			t.Fatal("changed project completed login")
		}
		select {
		case <-deadline:
			t.Fatal("changed project login did not finish")
		case <-time.After(20 * time.Millisecond):
		}
	}
	projectKey := oauthKey(workspace, true, serverSpec{Name: "shared", URL: projectURL})
	if _, found, _ := provider.authStore.read(projectKey); found {
		t.Fatal("changed project kept OAuth credential")
	}
}

func TestOAuth_registrationPriorityAndFallback(t *testing.T) {
	var remote *httptest.Server
	var registrations atomic.Int32
	var cimdSupported atomic.Bool
	cimdSupported.Store(true)
	remote = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+remote.URL+`/.well-known/oauth-protected-resource/mcp"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": remote.URL + "/mcp", "authorization_servers": []string{remote.URL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": remote.URL, "authorization_endpoint": remote.URL + "/authorize",
				"token_endpoint": remote.URL + "/token", "registration_endpoint": remote.URL + "/register",
				"response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"},
				"client_id_metadata_document_supported": cimdSupported.Load()})
		case "/register":
			registrations.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "registered-client", "token_endpoint_auth_method": "none"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	for _, mode := range []string{"preregistered", "cimd", "cimd-fallback"} {
		t.Run(mode, func(t *testing.T) {
			cimdSupported.Store(mode != "cimd-fallback")
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			redirect := "http://" + listener.Addr().String() + "/oauth/callback"
			listener.Close()
			config := serverConfig{Type: "http", URL: remote.URL + "/mcp", OAuthRedirectURL: redirect}
			wantClient := "known-client"
			if mode == "preregistered" {
				config.OAuthClientID = wantClient
			} else {
				wantClient = "https://example.com/harness-client.json"
				config.OAuthClientIDMetadataURL = wantClient
				if mode == "cimd-fallback" {
					wantClient = "registered-client"
				}
			}
			provider, err := newProvider(t.Context(), configFile{MCPServers: map[string]serverConfig{"demo": config}}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer provider.Close()
			provider.authStore.memoryOnly = true
			task, err := provider.StartOAuth("", "global", "demo")
			if err != nil {
				t.Fatal(err)
			}
			authorizeURL, err := url.Parse(task.URL)
			if err != nil {
				t.Fatal(err)
			}
			if got := authorizeURL.Query().Get("client_id"); got != wantClient {
				t.Fatalf("client ID = %q", got)
			}
			if got := authorizeURL.Query().Get("redirect_uri"); got != redirect {
				t.Fatalf("redirect = %q", got)
			}
			if _, err := provider.CancelOAuth(task.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
	if registrations.Load() != 1 {
		t.Fatalf("DCR fallback registrations = %d", registrations.Load())
	}
}

func TestOAuth_forbiddenOnlyStepsUpForInsufficientScope(t *testing.T) {
	store := newOAuthStore()
	store.memoryOnly = true
	handler := &oauthCredentialHandler{store: store, key: "key"}
	for _, tc := range []struct {
		header, state string
		scopes        int
	}{
		{`Bearer error="insufficient_scope", scope="read write"`, "insufficient-scope", 2},
		{`Bearer error="access_denied"`, "failed", 2},
	} {
		response := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Www-Authenticate": []string{tc.header}}, Body: io.NopCloser(strings.NewReader(""))}
		err := handler.Authorize(t.Context(), nil, response)
		var challenge *oauthChallenge
		if !errors.As(err, &challenge) || challenge.state != tc.state || len(store.scopes("key")) != tc.scopes {
			t.Fatalf("403 %q = %v, scopes=%v", tc.header, err, store.scopes("key"))
		}
	}
}

// 跨来源跳转不能带走 MCP access token 或令牌刷新请求中的 refresh token。
func TestOAuth_redirectsDoNotLeakCredentials(t *testing.T) {
	var received atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	handler := &oauthCredentialHandler{source: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access-secret"})}
	_, err := connectServer(t.Context(), serverSpec{Name: "demo", Transport: transportHTTP, URL: redirect.URL}, handler)
	if err == nil || received.Load() != 0 {
		t.Fatalf("MCP redirect: err=%v, target requests=%d", err, received.Load())
	}

	store := newOAuthStore()
	store.memoryOnly = true
	record := oauthRecord{Resource: redirect.URL, TokenURL: redirect.URL,
		Token: oauth2.Token{AccessToken: "old", RefreshToken: "refresh-secret", Expiry: time.Now().Add(-time.Minute)}}
	_, epoch, err := store.issue("demo", 0, record)
	if err != nil {
		t.Fatal(err)
	}
	source := &savedTokenSource{store: store, key: "demo", version: 0, epoch: epoch, record: record, client: oauthHTTPClient(redirect.URL)}
	_, err = source.Token()
	if err == nil || received.Load() != 0 {
		t.Fatalf("token redirect: err=%v, target requests=%d", err, received.Load())
	}
}

// 未提供 expires_in 的令牌在服务端失效后，401 仍应使用已有刷新令牌。
func TestOAuth_unauthorizedRefreshesTokenWithoutExpiry(t *testing.T) {
	var refreshes atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh token = %q", r.Form.Get("refresh_token"))
		}
		refreshes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	store := newOAuthStore()
	store.memoryOnly = true
	record := oauthRecord{Resource: tokenServer.URL, TokenURL: tokenServer.URL,
		Token: oauth2.Token{AccessToken: "old-access", RefreshToken: "old-refresh"}}
	_, epoch, err := store.issue("demo", 0, record)
	if err != nil {
		t.Fatal(err)
	}
	source := &savedTokenSource{store: store, key: "demo", version: 0, epoch: epoch, record: record, client: oauthHTTPClient(tokenServer.URL)}
	handler := &oauthCredentialHandler{store: store, key: "demo", source: source}
	request, err := http.NewRequest(http.MethodPost, tokenServer.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer old-access")
	response := &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}
	if err := handler.Authorize(t.Context(), request, response); err != nil {
		t.Fatalf("401 refresh: %v", err)
	}
	token, err := source.Token()
	if err != nil || token.AccessToken != "new-access" || refreshes.Load() != 1 {
		t.Fatalf("refreshed token=%#v, err=%v, requests=%d", token, err, refreshes.Load())
	}
}
