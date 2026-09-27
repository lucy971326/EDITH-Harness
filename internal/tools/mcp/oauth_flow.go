package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

var ErrOAuthUntrusted = errors.New("mcp: project OAuth server needs trust")

// 数据。设置页只接收授权网址、任务进度和安全提示。
type OAuthTaskView struct {
	ID      string `json:"id"`
	URL     string `json:"url,omitempty"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Storage string `json:"storage,omitempty"`
}

type oauthTask struct {
	mu            sync.Mutex
	view          OAuthTaskView
	key           string
	cancel        context.CancelFunc
	done          chan struct{}
	url           chan string
	callback      chan *auth.AuthorizationResult
	callbackError chan error
}

func (t *oauthTask) snapshot() OAuthTaskView { t.mu.Lock(); defer t.mu.Unlock(); return t.view }
func (t *oauthTask) update(state, message, storage string) {
	t.mu.Lock()
	t.view.State, t.view.Message, t.view.Storage = state, message, storage
	t.mu.Unlock()
}

func newOAuthID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func (p *Provider) oauthEntry(workspace, scope, name string) (sourcePlan, plannedServer, error) {
	if scope == "global" {
		workspace = ""
	}
	plan, err := p.plan(workspace)
	if err != nil {
		return sourcePlan{}, plannedServer{}, err
	}
	entries := plan.entries
	if scope == "global" {
		entries = plan.global
	}
	for _, entry := range entries {
		if entry.name != name || (scope == "project") != entry.project {
			continue
		}
		if entry.spec == nil {
			base := p.launchDir
			if entry.project {
				base = workspace
			}
			spec, err := normalizeServer(entry.name, entry.config, base)
			if err != nil {
				if entry.config.isEnabled() {
					return sourcePlan{}, plannedServer{}, fmt.Errorf("%w: Server 配置无效", ErrInvalid)
				}
				spec = oauthIdentitySpec(entry.name, entry.config)
				spec.Transport = entry.config.Type
			}
			entry.spec = &spec
		}
		if entry.spec.Transport != transportHTTP {
			return sourcePlan{}, plannedServer{}, fmt.Errorf("%w: 不是 HTTP Server", ErrInvalid)
		}
		return plan, entry, nil
	}
	return sourcePlan{}, plannedServer{}, ErrMissing
}

// TrustProjectOAuth 在设置页显示项目来源并确认后，保存当前摘要；随后再次检查版本。
func (p *Provider) TrustProjectOAuth(workspace, version string) error {
	if workspace == "" || p.approvals == nil {
		return ErrInvalid
	}
	plan, err := p.plan(workspace)
	if err != nil {
		return err
	}
	if plan.version != version || plan.projectError != "" {
		return ErrChanged
	}
	realPath, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return ErrInvalid
	}
	return p.approvals.TrustMCPConfigFromSettings(realPath, plan.projectDigest)
}

// StartOAuth 创建独立登录任务；只等到 SDK 提供授权网址，不等待用户登录。
func (p *Provider) StartOAuth(workspace, scope, name string) (OAuthTaskView, error) {
	if scope != "global" && scope != "project" {
		return OAuthTaskView{}, ErrInvalid
	}
	if scope == "global" {
		workspace = ""
	}
	plan, entry, err := p.oauthEntry(workspace, scope, name)
	if err != nil {
		return OAuthTaskView{}, err
	}
	if !entry.config.isEnabled() {
		return OAuthTaskView{}, fmt.Errorf("%w: Server 未启用", ErrInvalid)
	}
	if entry.project {
		if p.approvals == nil {
			return OAuthTaskView{}, ErrOAuthUntrusted
		}
		realPath, err := filepath.EvalSymlinks(workspace)
		if err != nil {
			return OAuthTaskView{}, ErrInvalid
		}
		trusted, err := p.approvals.IsMCPConfigTrusted(realPath, plan.projectDigest)
		if err != nil {
			return OAuthTaskView{}, err
		}
		if !trusted {
			return OAuthTaskView{}, ErrOAuthUntrusted
		}
		workspace = realPath
	}
	id, err := newOAuthID()
	if err != nil {
		return OAuthTaskView{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	task := &oauthTask{view: OAuthTaskView{ID: id, State: "preparing"}, key: oauthKey(workspace, entry.project, *entry.spec), cancel: cancel, done: make(chan struct{}), url: make(chan string, 1), callback: make(chan *auth.AuthorizationResult, 1), callbackError: make(chan error, 1)}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		cancel()
		return OAuthTaskView{}, context.Canceled
	}
	for _, existing := range p.authTasks {
		v := existing.snapshot()
		if existing.key == task.key && (v.State == "preparing" || v.State == "waiting" || v.State == "connecting") {
			p.mu.Unlock()
			cancel()
			return v, nil
		}
	}
	p.authTasks[id] = task
	p.wg.Add(1)
	p.mu.Unlock()
	go func() {
		defer p.wg.Done()
		defer close(task.done)
		defer cancel()
		p.runOAuth(ctx, workspace, plan, entry, task)
		time.AfterFunc(10*time.Minute, func() { p.mu.Lock(); delete(p.authTasks, id); p.mu.Unlock() })
	}()
	select {
	case address := <-task.url:
		view := task.snapshot()
		view.URL = address
		return view, nil
	case <-task.done:
		return task.snapshot(), nil
	case <-time.After(30 * time.Second):
		cancel()
		return OAuthTaskView{}, fmt.Errorf("%w: 获取授权网址超时", ErrInvalid)
	}
}

func (p *Provider) runOAuth(ctx context.Context, workspace string, plan sourcePlan, entry plannedServer, task *oauthTask) {
	spec := *entry.spec
	key := oauthKey(workspace, entry.project, spec)
	newCredentialSaved := false
	var newCredentialEpoch uint64
	priorRecord, priorFound, priorErr := p.authStore.read(key)
	if priorErr != nil {
		task.update("failed", "已保存的 OAuth 凭据无法读取", "")
		return
	}
	version := p.authStore.version(key)
	defer func() {
		if ctx.Err() != nil && newCredentialSaved && task.snapshot().State != "complete" && p.authStore.credentialEpoch(key) == newCredentialEpoch {
			if priorFound && p.authStore.version(key) == version {
				_ = p.authStore.restoreIfEpoch(key, version, newCredentialEpoch, priorRecord)
			} else {
				_ = p.authStore.retireIfEpoch(key, newCredentialEpoch)
			}
		}
	}()
	client := oauthHTTPClient(spec.URL)
	challenge := probeOAuth(ctx, spec, client)
	discovery, err := discoverOAuth(ctx, spec, challenge, client)
	if err != nil {
		task.update("failed", err.Error(), "")
		return
	}
	redirect, listener, err := oauthCallbackListener(spec)
	if err != nil {
		task.update("failed", "本机回调地址不可用或未配置", "")
		return
	}
	defer listener.Close()
	callbackURL := mustParseURL(redirect)
	server := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	var expectedState string
	var stateMu sync.Mutex
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != callbackURL.Path || r.Host != callbackURL.Host {
			http.NotFound(w, r)
			return
		}
		stateMu.Lock()
		expected := expectedState
		stateMu.Unlock()
		if expected == "" || r.URL.Query().Get("state") != expected {
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}
		if value := r.URL.Query().Get("error"); value != "" {
			select {
			case task.callbackError <- errors.New("用户取消或授权服务拒绝登录"):
			default:
			}
			_, _ = io.WriteString(w, "授权未完成，可返回 Harness 重试。")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing OAuth code", http.StatusBadRequest)
			return
		}
		result := &auth.AuthorizationResult{Code: code, State: expected, Iss: r.URL.Query().Get("iss")}
		select {
		case task.callback <- result:
			_, _ = io.WriteString(w, "授权已完成，可以返回 Harness。")
		default:
			http.Error(w, "OAuth callback already received", http.StatusConflict)
		}
	})
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	loginCtx := ctx
	config := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL: redirect, Client: client, RequestRefreshToken: true, AcceptUnadvertisedIss: true,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			address := mustParseURL(args.URL)
			if err := safeOAuthURL(args.URL, loopbackHost(mustParseURL(spec.URL).Hostname())); err != nil {
				return nil, err
			}
			authEndpoint := mustParseURL(discovery.authURL)
			if address.Scheme != authEndpoint.Scheme || address.Host != authEndpoint.Host || address.Path != authEndpoint.Path || address.Query().Get("resource") != discovery.resource {
				return nil, errors.New("授权地址与已验证的服务不一致")
			}
			for key, values := range authEndpoint.Query() {
				if strings.Join(address.Query()[key], "\x00") != strings.Join(values, "\x00") {
					return nil, errors.New("授权地址参数与已验证的服务不一致")
				}
			}
			state := address.Query().Get("state")
			if state == "" {
				return nil, errors.New("授权网址缺少 state")
			}
			stateMu.Lock()
			expectedState = state
			stateMu.Unlock()
			task.update("waiting", "等待浏览器授权", "")
			task.mu.Lock()
			task.view.URL = args.URL
			task.mu.Unlock()
			select {
			case task.url <- args.URL:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			select {
			case result := <-task.callback:
				return result, nil
			case err := <-task.callbackError:
				return nil, err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		NewTokenSource: func(_ context.Context, cfg *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			if loginCtx.Err() != nil || p.authStore.version(key) != version {
				return nil, context.Canceled
			}
			if cfg.Endpoint.AuthURL != discovery.authURL || cfg.Endpoint.TokenURL != discovery.tokenURL {
				return nil, errors.New("令牌地址与已验证的授权服务不一致")
			}
			current, err := p.plan(workspace)
			if err != nil || current.version != plan.version {
				return nil, ErrChanged
			}
			record := oauthRecord{Resource: discovery.resource, Issuer: discovery.issuer, ClientID: cfg.ClientID,
				ClientSecret: cfg.ClientSecret, AuthURL: cfg.Endpoint.AuthURL, TokenURL: cfg.Endpoint.TokenURL,
				AuthStyle: cfg.Endpoint.AuthStyle, Scopes: append([]string(nil), cfg.Scopes...), Token: *token}
			persisted, epoch, err := p.authStore.issue(key, version, record)
			if err != nil {
				return nil, err
			}
			newCredentialSaved = true
			newCredentialEpoch = epoch
			storage := "memory"
			if persisted {
				storage = "file"
			}
			task.update("connecting", "授权已完成，正在连接", storage)
			return &savedTokenSource{store: p.authStore, key: key, version: version, epoch: epoch, record: record, client: client}, nil
		},
	}
	if spec.OAuthClientID != "" {
		credentials := &oauthex.ClientCredentials{ClientID: spec.OAuthClientID, Issuer: discovery.issuer}
		if spec.OAuthClientSecretEnv != "" {
			secret := os.Getenv(spec.OAuthClientSecretEnv)
			if secret == "" {
				task.update("failed", "OAuth 客户端密钥环境变量未设置", "")
				return
			}
			credentials.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: secret}
		}
		config.PreregisteredClient = credentials
	} else {
		if spec.OAuthClientIDMetadataURL != "" {
			if err := safeOAuthURL(spec.OAuthClientIDMetadataURL, false); err != nil {
				task.update("failed", "CIMD 地址无效", "")
				return
			}
			config.ClientIDMetadataDocumentConfig = &auth.ClientIDMetadataDocumentConfig{URL: spec.OAuthClientIDMetadataURL}
		}
		if discovery.registrationURL != "" {
			config.DynamicClientRegistrationConfig = &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
				RedirectURIs: []string{redirect}, ClientName: "Harness", TokenEndpointAuthMethod: "none",
				GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"},
			}}
		}
		if (spec.OAuthClientIDMetadataURL == "" || !discovery.cimdSupported) && discovery.registrationURL == "" {
			task.mu.Lock()
			task.view.State = "failed"
			task.view.Message = "此服务不支持自动注册；请配置已注册的 Client ID、回调地址及所需密钥环境变量，或改用访问令牌"
			task.view.Reason = "client-registration-required"
			task.mu.Unlock()
			return
		}
	}
	handler, err := auth.NewAuthorizationCodeHandler(config)
	if err != nil {
		task.update("failed", "OAuth 客户端配置无效", "")
		return
	}
	// SDK 用请求 URL 校验 PRM.resource；这里固定为已验证的规范资源地址。
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, discovery.resource, nil)
	resp := &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}
	var scopes []string
	if challenge != nil {
		parsed, _ := oauthex.ParseWWWAuthenticate(challenge.Header.Values("WWW-Authenticate"))
		for _, item := range parsed {
			if item.Scheme == "bearer" {
				scopes = append(scopes, strings.Fields(item.Params["scope"])...)
				break
			}
		}
	}
	_, saved, _ := p.authStore.read(key)
	if saved {
		record, _, _ := p.authStore.read(key)
		scopes = append(scopes, record.Scopes...)
		scopes = append(scopes, p.authStore.scopes(key)...)
	}
	header := "Bearer"
	if len(scopes) > 0 {
		header += ` scope="` + strings.Join(uniqueScopes(scopes), " ") + `"`
	}
	if len(scopes) > 0 {
		header += ","
	}
	header += ` resource_metadata="` + discovery.metadataURL + `"`
	resp.Header.Set("WWW-Authenticate", header)
	err = handler.Authorize(ctx, req, resp)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			task.update("cancelled", "登录已取消", "")
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			task.update("failed", "登录超时", "")
		} else {
			task.update("failed", "授权或换取令牌失败", "")
		}
		return
	}
	if p.authStore.version(key) != version {
		task.update("cancelled", "登录已取消", "")
		return
	}
	current, err := p.plan(workspace)
	if err != nil || current.version != plan.version {
		_ = p.authStore.retireIfEpoch(key, newCredentialEpoch)
		task.update("failed", "配置已改变，请重新登录", "")
		return
	}
	if entry.project {
		p.invalidate(workspace)
	} else {
		p.invalidateAll()
	}
	connectCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	state, err := p.state(connectCtx, workspace)
	cancel()
	if err != nil {
		task.update("failed", "授权成功，但重新连接失败", "")
		return
	}
	p.mu.Lock()
	connected := state.status[entry.name].state == "connected"
	p.mu.Unlock()
	if !connected {
		task.update("failed", "授权成功，但 Server 仍未连接", "")
		return
	}
	storage := task.snapshot().Storage
	message := "已连接"
	if storage == "memory" {
		message = "已连接；凭据仅在当前进程中，重启后需重新登录"
	}
	task.update("complete", message, storage)
}

func uniqueScopes(values []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func oauthCallbackListener(spec serverSpec) (string, net.Listener, error) {
	if spec.OAuthClientID == "" && spec.OAuthClientIDMetadataURL == "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", nil, err
		}
		return "http://" + listener.Addr().String() + "/oauth/callback", listener, nil
	}
	redirect := mustParseURL(spec.OAuthRedirectURL)
	if redirect.Scheme != "http" || !loopbackHost(redirect.Hostname()) || redirect.Port() == "" || redirect.Path == "" || redirect.RawQuery != "" || redirect.Fragment != "" {
		return "", nil, ErrInvalid
	}
	listener, err := net.Listen("tcp", redirect.Host)
	return spec.OAuthRedirectURL, listener, err
}

func (p *Provider) OAuthStatus(id string) (OAuthTaskView, error) {
	p.mu.Lock()
	task := p.authTasks[id]
	p.mu.Unlock()
	if task == nil {
		return OAuthTaskView{}, ErrMissing
	}
	return task.snapshot(), nil
}

func (p *Provider) CancelOAuth(id string) (OAuthTaskView, error) {
	p.mu.Lock()
	task := p.authTasks[id]
	p.mu.Unlock()
	if task == nil {
		return OAuthTaskView{}, ErrMissing
	}
	task.cancel()
	<-task.done
	return task.snapshot(), nil
}

func (p *Provider) LogoutOAuth(workspace, scope, name string) (SettingsView, error) {
	_, entry, err := p.oauthEntry(workspace, scope, name)
	if err != nil {
		return SettingsView{}, err
	}
	if entry.project {
		workspace, err = filepath.EvalSymlinks(workspace)
		if err != nil {
			return SettingsView{}, ErrInvalid
		}
	}
	key := oauthKey(workspace, entry.project, *entry.spec)
	removeErr := p.revokeOAuthKey(key)
	if entry.project {
		p.invalidate(workspace)
	} else {
		p.invalidateAll()
	}
	if removeErr != nil {
		return SettingsView{}, fmt.Errorf("%w: %v", ErrOAuthCredentialCleanup, removeErr)
	}
	return p.ReadSettings(workspace)
}

func (p *Provider) revokeOAuthKey(key string) error {
	p.cancelOAuthTasks(key)
	return p.authStore.delete(key)
}

func (p *Provider) retireOAuthKey(key string) error {
	p.cancelOAuthTasks(key)
	return p.authStore.retire(key)
}

func (p *Provider) cancelOAuthTasks(key string) {
	p.mu.Lock()
	for _, task := range p.authTasks {
		if task.key == key {
			task.cancel()
		}
	}
	p.mu.Unlock()
}
