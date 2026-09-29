package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrLoginExpired = errors.New("model OAuth login expired")
var formClient = &http.Client{Timeout: 30 * time.Second}

// 数据。两个设备码供应商的公开端点与客户端标识。
type FormDevice struct {
	Name               string
	ClientID           string
	DeviceURL          string
	TokenURL           string
	Scope              string
	VerificationDomain string
	// xAI 的刷新响应可能沿用旧 refresh token。
	ReuseRefresh bool
}

type formResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
	AccessToken             string `json:"access_token"`
	RefreshToken            string `json:"refresh_token"`
	Error                   string `json:"error"`
}

func (d *FormDevice) post(ctx context.Context, endpoint string, fields url.Values) (formResponse, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(fields.Encode()))
	if err != nil {
		return formResponse{}, 0, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := formClient.Do(request)
	if err != nil {
		return formResponse{}, 0, fmt.Errorf("%s 授权请求失败: %w", d.Name, err)
	}
	defer response.Body.Close()
	var result formResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return formResponse{}, 0, fmt.Errorf("%s 授权服务返回无效 JSON (HTTP %d)", d.Name, response.StatusCode)
	}
	return result, response.StatusCode, nil
}

func (d *FormDevice) Begin(ctx context.Context) (Challenge, error) {
	fields := url.Values{"client_id": {d.ClientID}}
	if d.Scope != "" {
		fields.Set("scope", d.Scope)
	}
	result, status, err := d.post(ctx, d.DeviceURL, fields)
	if err != nil {
		return Challenge{}, err
	}
	if status < 200 || status >= 300 {
		return Challenge{}, fmt.Errorf("%s 设备授权失败 (HTTP %d)", d.Name, status)
	}
	address := result.VerificationURIComplete
	if address == "" {
		address = result.VerificationURI
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() == "" {
		return Challenge{}, fmt.Errorf("%s 返回不可信的授权地址", d.Name)
	}
	// 只允许供应商自己的授权页，防止设备码服务响应把用户导向其他网站。
	host := parsed.Hostname()
	expected := d.VerificationDomain
	if host != expected && !strings.HasSuffix(host, "."+expected) {
		return Challenge{}, fmt.Errorf("%s 返回不可信的授权地址", d.Name)
	}
	if result.DeviceCode == "" || result.UserCode == "" || result.ExpiresIn <= 0 {
		return Challenge{}, fmt.Errorf("%s 设备授权响应不完整", d.Name)
	}
	return Challenge{DeviceCode: result.DeviceCode, UserCode: result.UserCode, URL: address,
		Interval: time.Duration(result.Interval) * time.Second, Expires: time.Duration(result.ExpiresIn) * time.Second}, nil
}

func (d *FormDevice) credential(result formResponse, previous string) (*Credential, error) {
	refresh := result.RefreshToken
	if refresh == "" && d.ReuseRefresh {
		refresh = previous
	}
	expires := result.ExpiresIn
	if expires <= 0 && d.ReuseRefresh {
		expires = 3600
	}
	if result.AccessToken == "" || refresh == "" || expires <= 0 {
		return nil, fmt.Errorf("%s 令牌响应不完整", d.Name)
	}
	return &Credential{Access: result.AccessToken, Refresh: refresh,
		Expires: time.Now().Add(time.Duration(expires) * time.Second)}, nil
}

func (d *FormDevice) Poll(ctx context.Context, code string) (DeviceResult, error) {
	result, status, err := d.post(ctx, d.TokenURL, url.Values{
		"client_id": {d.ClientID}, "device_code": {code},
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
	})
	if err != nil {
		return DeviceResult{}, err
	}
	if status >= 200 && status < 300 {
		credential, err := d.credential(result, "")
		return DeviceResult{Credential: credential}, err
	}
	switch result.Error {
	case "authorization_pending":
		return DeviceResult{}, nil
	case "slow_down":
		return DeviceResult{SlowDown: true, Interval: time.Duration(result.Interval) * time.Second}, nil
	case "access_denied", "authorization_denied":
		return DeviceResult{}, fmt.Errorf("%s 授权被拒绝", d.Name)
	case "expired_token":
		return DeviceResult{}, fmt.Errorf("%s 设备码已过期", d.Name)
	default:
		return DeviceResult{}, fmt.Errorf("%s 授权失败 (HTTP %d)", d.Name, status)
	}
}

func (d *FormDevice) Refresh(ctx context.Context, previous Credential) (Credential, error) {
	result, status, err := d.post(ctx, d.TokenURL, url.Values{
		"client_id": {d.ClientID}, "refresh_token": {previous.Refresh}, "grant_type": {"refresh_token"},
	})
	if err != nil {
		return Credential{}, err
	}
	if status < 200 || status >= 300 {
		if result.Error == "invalid_grant" || status == http.StatusUnauthorized {
			return Credential{}, fmt.Errorf("%w: %s 登录已失效", ErrLoginExpired, d.Name)
		}
		return Credential{}, fmt.Errorf("%s 刷新登录失败 (HTTP %d)", d.Name, status)
	}
	credential, err := d.credential(result, previous.Refresh)
	if err != nil {
		return Credential{}, err
	}
	return *credential, nil
}
