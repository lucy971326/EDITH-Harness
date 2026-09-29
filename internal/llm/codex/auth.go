package codex

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"harness/internal/persist"
)

const (
	codexClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexRedirect     = "http://localhost:1455/auth/callback"
	codexAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	codexTokenURL     = "https://auth.openai.com/oauth/token"
	credentialFile    = "codex.json"
)

var ErrCallbackBusy = errors.New("llm: ChatGPT callback port is in use")

type credential struct {
	Access    string    `json:"access"`
	Refresh   string    `json:"refresh"`
	Expires   time.Time `json:"expires"`
	AccountID string    `json:"accountId"`
}

type authTask struct {
	view     View
	verifier string
	state    string
	server   *http.Server
	cancel   context.CancelFunc
	ctx      context.Context
}

// 活对象。订阅凭据与一个浏览器回调任务；同一把锁串行化刷新、退出与登录落盘。
type Auth struct {
	mu           sync.Mutex
	workers      sync.WaitGroup
	files        *persist.Files
	credential   *credential
	loadError    string
	task         *authTask
	closed       bool
	client       *http.Client
	tokenURL     string
	authorizeURL string
	baseURL      string
}

// New 读取本机订阅凭据，损坏凭据由设置页提示重新登录。
func New(files *persist.Files) (*Auth, error) {
	scope, err := files.Scope("model-auth")
	if err != nil {
		return nil, err
	}
	auth := &Auth{files: scope, client: &http.Client{Timeout: 20 * time.Second},
		tokenURL: codexTokenURL, authorizeURL: codexAuthorizeURL,
		baseURL: "https://chatgpt.com/backend-api/codex"}
	body, err := scope.Read(credentialFile)
	if errors.Is(err, os.ErrNotExist) {
		return auth, nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := openOAuthCredential(body)
	if err == nil {
		var credential credential
		err = json.Unmarshal(plain, &credential)
		clear(plain)
		if err == nil && credential.Access != "" && credential.Refresh != "" && credential.AccountID != "" {
			auth.credential = &credential
			return auth, nil
		}
	}
	auth.loadError = "已保存的 ChatGPT 登录信息无法读取，请重新登录"
	return auth, nil
}

func (a *Auth) Authenticated() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.credential != nil
}

func (a *Auth) viewLocked() View {
	if a.task != nil {
		view := a.task.view
		view.Authenticated = a.credential != nil
		return view
	}
	if a.credential != nil {
		return View{Authenticated: true, State: "connected"}
	}
	return View{State: "disconnected", Message: a.loadError}
}

func (a *Auth) Status() View {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.viewLocked()
}

func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (a *Auth) Start() (View, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return View{}, fmt.Errorf("llm: OAuth service is closed")
	}
	if a.task != nil && (a.task.view.State == "waiting" || a.task.view.State == "connecting") {
		return a.viewLocked(), nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:1455")
	if err != nil {
		return View{}, fmt.Errorf("%w: %v", ErrCallbackBusy, err)
	}
	verifier, err := randomHex(32)
	if err != nil {
		_ = listener.Close()
		return View{}, err
	}
	state, err := randomHex(16)
	if err != nil {
		_ = listener.Close()
		return View{}, err
	}
	challenge := sha256.Sum256([]byte(verifier))
	address, err := url.Parse(a.authorizeURL)
	if err != nil {
		_ = listener.Close()
		return View{}, err
	}
	query := address.Query()
	query.Set("response_type", "code")
	query.Set("client_id", codexClientID)
	query.Set("redirect_uri", codexRedirect)
	query.Set("scope", "openid profile email offline_access")
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)
	query.Set("id_token_add_organizations", "true")
	query.Set("codex_cli_simplified_flow", "true")
	query.Set("originator", "edith")
	address.RawQuery = query.Encode()
	ctx, cancel := context.WithCancel(context.Background())
	task := &authTask{view: View{State: "waiting", URL: address.String()},
		verifier: verifier, state: state, ctx: ctx, cancel: cancel}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) { a.callback(task, w, r) })
	task.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	a.task = task
	a.workers.Add(2)
	go func() { defer a.workers.Done(); _ = task.server.Serve(listener) }()
	go func() {
		defer a.workers.Done()
		select {
		case <-time.After(10 * time.Minute):
			a.mu.Lock()
			if a.task == task && task.view.State == "waiting" {
				task.view = View{State: "failed", Message: "登录超时，请重试"}
				task.cancel()
				_ = task.server.Close()
			}
			a.mu.Unlock()
		case <-ctx.Done():
		}
	}()
	return a.viewLocked(), nil
}

func (a *Auth) callback(task *authTask, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/auth/callback" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("state") != task.state {
		http.Error(w, "授权状态不匹配", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "缺少授权码", http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	if a.task != task || task.view.State != "waiting" {
		a.mu.Unlock()
		http.Error(w, "授权已结束", http.StatusGone)
		return
	}
	task.view = View{State: "connecting"}
	a.workers.Add(1)
	a.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, "<!doctype html><meta charset=utf-8><title>EDITH</title><p>授权已收到，可以返回 EDITH。</p>")
	go func() { defer a.workers.Done(); a.finish(task, code) }()
}

func (a *Auth) finish(task *authTask, code string) {
	defer func() { _ = task.server.Close(); task.cancel() }()
	credential, err := a.exchange(task.ctx, url.Values{
		"grant_type": {"authorization_code"}, "client_id": {codexClientID},
		"code": {code}, "code_verifier": {task.verifier}, "redirect_uri": {codexRedirect},
	})
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.task != task || task.ctx.Err() != nil {
		return
	}
	if err == nil {
		err = a.saveLocked(credential)
	}
	if err != nil {
		task.view = View{State: "failed", Message: err.Error()}
		return
	}
	a.credential = credential
	a.loadError = ""
	task.view = View{Authenticated: true, State: "complete"}
}

func (a *Auth) exchange(ctx context.Context, values url.Values) (*credential, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("llm: ChatGPT token request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: ChatGPT token request failed (HTTP %d)", response.StatusCode)
	}
	var result struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("llm: invalid ChatGPT token response: %w", err)
	}
	if result.Access == "" || result.Refresh == "" || result.Expires <= 0 {
		return nil, fmt.Errorf("llm: incomplete ChatGPT token response")
	}
	accountID, err := accountIDFromJWT(result.Access)
	if err != nil {
		return nil, err
	}
	return &credential{Access: result.Access, Refresh: result.Refresh,
		Expires: time.Now().Add(time.Duration(result.Expires) * time.Second), AccountID: accountID}, nil
}

func accountIDFromJWT(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("llm: ChatGPT token has no account identity")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("llm: invalid ChatGPT account identity")
	}
	var payload struct {
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Auth.AccountID == "" {
		return "", fmt.Errorf("llm: ChatGPT token has no account identity")
	}
	return payload.Auth.AccountID, nil
}

func (a *Auth) saveLocked(credential *credential) error {
	body, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	sealed, err := sealOAuthCredential(body)
	clear(body)
	if err != nil {
		return err
	}
	return a.files.Write(credentialFile, sealed)
}

func (a *Auth) token(ctx context.Context) (*credential, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.credential == nil {
		return nil, fmt.Errorf("llm: 请先登录 ChatGPT")
	}
	if time.Until(a.credential.Expires) > 2*time.Minute {
		copy := *a.credential
		return &copy, nil
	}
	updated, err := a.exchange(ctx, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {a.credential.Refresh}, "client_id": {codexClientID},
	})
	if err != nil {
		return nil, err
	}
	if err := a.saveLocked(updated); err != nil {
		return nil, err
	}
	a.credential = updated
	copy := *updated
	return &copy, nil
}

func (a *Auth) Cancel() View {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.task != nil && (a.task.view.State == "waiting" || a.task.view.State == "connecting") {
		a.task.view = View{State: "cancelled"}
		a.task.cancel()
		_ = a.task.server.Close()
	}
	return a.viewLocked()
}

func (a *Auth) Logout() (View, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.task != nil {
		a.task.cancel()
		_ = a.task.server.Close()
		a.task = nil
	}
	err := a.files.Remove(credentialFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return a.viewLocked(), err
	}
	a.credential = nil
	a.loadError = ""
	return a.viewLocked(), nil
}

func (a *Auth) Close() {
	a.mu.Lock()
	a.closed = true
	if a.task != nil {
		a.task.cancel()
		_ = a.task.server.Close()
		a.task = nil
	}
	a.mu.Unlock()
	a.workers.Wait()
}
