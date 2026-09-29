package xai

import (
	"context"

	"harness/internal/llm/oauth"
	"harness/internal/persist"
)

// 活对象。xAI 设备码登录与固定地址的 Responses 模型。
type Auth struct{ *oauth.DeviceAuth }

func New(files *persist.Files) (*Auth, error) {
	driver := &oauth.FormDevice{Name: "xAI", ClientID: "b1a00492-073a-47ea-816f-4c329264a828",
		DeviceURL: "https://auth.x.ai/oauth2/device/code", TokenURL: "https://auth.x.ai/oauth2/token",
		Scope: "openid profile email offline_access grok-cli:access api:access", ReuseRefresh: true,
		VerificationDomain: "x.ai"}
	auth, err := oauth.NewDeviceAuth(files, "xai", driver)
	if err != nil {
		return nil, err
	}
	return &Auth{DeviceAuth: auth}, nil
}

// RequestAuth 在发起请求前取得可用的设备码登录令牌。
func (a *Auth) RequestAuth(ctx context.Context) (oauth.Authorization, error) {
	token, err := a.Token(ctx)
	return oauth.Authorization{Token: token}, err
}
