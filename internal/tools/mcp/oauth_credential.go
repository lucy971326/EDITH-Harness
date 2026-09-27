package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

type oauthChallenge struct{ state, message string }

func (e *oauthChallenge) Error() string { return e.message }

func classifyOAuthResponse(resp *http.Response) (*oauthChallenge, []string) {
	state, message := "auth-required", "需要登录"
	if resp.StatusCode == http.StatusForbidden {
		state, message = "failed", "访问被拒绝"
	}
	var scopes []string
	challenges, _ := oauthex.ParseWWWAuthenticate(resp.Header.Values("WWW-Authenticate"))
	for _, item := range challenges {
		if item.Scheme != "bearer" {
			continue
		}
		if value := item.Params["scope"]; value != "" {
			scopes = strings.Fields(value)
		}
		if resp.StatusCode == http.StatusForbidden && item.Params["error"] == "insufficient_scope" {
			state, message = "insufficient-scope", "需要增加授权权限"
		}
	}
	return &oauthChallenge{state: state, message: message}, scopes
}

type oauthCredentialHandler struct {
	store   *oauthStore
	key     string
	version uint64
	source  oauth2.TokenSource
}

func (h *oauthCredentialHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return h.source, nil
}
func (h *oauthCredentialHandler) Authorize(_ context.Context, req *http.Request, resp *http.Response) error {
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	challenge, scopes := classifyOAuthResponse(resp)
	if challenge.state == "insufficient-scope" {
		h.store.requireScopes(h.key, scopes)
	}
	if resp.StatusCode == http.StatusUnauthorized && req != nil && strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
		if source, ok := h.source.(*savedTokenSource); ok {
			_, err := source.token(true, req.Header.Get("Authorization"))
			return err // SDK 在成功刷新后只重试本次 MCP 请求一次。
		}
	}
	return challenge
}

func (p *Provider) credentialHandler(ctx context.Context, workspace string, entry plannedServer) (*oauthCredentialHandler, error) {
	if entry.project {
		resolved, err := filepath.EvalSymlinks(workspace)
		if err != nil {
			return nil, errors.New("项目路径不可用")
		}
		workspace = resolved
	}
	key := oauthKey(workspace, entry.project, *entry.spec)
	handler := &oauthCredentialHandler{store: p.authStore, key: key, version: p.authStore.version(key)}
	record, exists, err := p.authStore.read(key)
	if err != nil {
		return nil, errors.New("已保存的 OAuth 凭据无法读取")
	}
	if !exists {
		return handler, nil
	}
	client := oauthHTTPClient(entry.spec.URL)
	challenge := probeOAuth(ctx, *entry.spec, client)
	discovery, err := discoverOAuth(ctx, *entry.spec, challenge, client)
	if err != nil {
		return nil, err
	}
	if record.Resource != discovery.resource || record.Issuer != discovery.issuer ||
		record.AuthURL != discovery.authURL || record.TokenURL != discovery.tokenURL {
		return nil, errors.New("OAuth 授权服务或资源已改变，请重新登录")
	}
	handler.source = &savedTokenSource{store: p.authStore, key: key, version: handler.version, epoch: p.authStore.credentialEpoch(key), record: record, client: client}
	return handler, nil
}

// 活对象。串行刷新并保存轮换令牌；每次返回前核对退出登录代数。
type savedTokenSource struct {
	mu      sync.Mutex
	store   *oauthStore
	key     string
	version uint64
	epoch   uint64
	record  oauthRecord
	client  *http.Client
}

func (s *savedTokenSource) Token() (*oauth2.Token, error) {
	return s.token(false, "")
}

func (s *savedTokenSource) token(force bool, rejectedAuthorization string) (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store.version(s.key) != s.version {
		return nil, &oauthChallenge{state: "auth-required", message: "OAuth 已退出登录"}
	}
	if s.record.Token.Valid() && (!force || "Bearer "+s.record.Token.AccessToken != rejectedAuthorization) {
		token := s.record.Token
		return &token, nil
	}
	// 同一 Server 的旧、新连接可能同时持有 TokenSource，轮换刷新令牌必须串行。
	refreshMu := s.store.refreshLock(s.key)
	refreshMu.Lock()
	defer refreshMu.Unlock()
	if s.store.version(s.key) != s.version {
		return nil, &oauthChallenge{state: "auth-required", message: "OAuth 已退出登录"}
	}
	if s.store.credentialEpoch(s.key) == s.epoch {
		latest, found, err := s.store.read(s.key)
		if err != nil {
			return nil, errors.New("已保存的 OAuth 凭据无法读取")
		}
		if found {
			s.record = latest
			if latest.Token.Valid() && (!force || "Bearer "+latest.Token.AccessToken != rejectedAuthorization) {
				token := latest.Token
				return &token, nil
			}
		}
	}
	if s.record.Token.RefreshToken == "" {
		return nil, &oauthChallenge{state: "auth-required", message: "OAuth 登录已过期，请重新登录"}
	}
	values := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {s.record.Token.RefreshToken}, "resource": {s.record.Resource}}
	request, err := http.NewRequest(http.MethodPost, s.record.TokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, errors.New("OAuth 刷新地址无效")
	}
	if s.record.ClientSecret != "" && s.record.AuthStyle != oauth2.AuthStyleInParams {
		request.SetBasicAuth(s.record.ClientID, s.record.ClientSecret)
	} else {
		values.Set("client_id", s.record.ClientID)
		if s.record.ClientSecret != "" {
			values.Set("client_secret", s.record.ClientSecret)
		}
		request.Body = io.NopCloser(strings.NewReader(values.Encode()))
		request.ContentLength = int64(len(values.Encode()))
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(request)
	if err != nil {
		return nil, errors.New("OAuth 令牌刷新失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &oauthChallenge{state: "auth-required", message: "OAuth 令牌刷新被拒绝，请重新登录"}
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil || payload.AccessToken == "" {
		return nil, errors.New("OAuth 令牌刷新响应无效")
	}
	if payload.RefreshToken == "" {
		payload.RefreshToken = s.record.Token.RefreshToken
	}
	s.record.Token = oauth2.Token{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, TokenType: payload.TokenType}
	if payload.ExpiresIn > 0 {
		s.record.Token.Expiry = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	if s.store.version(s.key) != s.version {
		return nil, &oauthChallenge{state: "auth-required", message: "OAuth 已退出登录"}
	}
	if err := s.store.refresh(s.key, s.version, s.epoch, s.record); err != nil {
		return nil, fmt.Errorf("mcp: 保存刷新令牌失败")
	}
	token := s.record.Token
	return &token, nil
}
