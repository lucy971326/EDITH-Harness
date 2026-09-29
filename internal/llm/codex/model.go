package codex

import (
	"context"
	"harness/internal/llm/oauth"
)

// RequestAuth 刷新并持久化凭据后，交给当前请求使用。
func (a *Auth) RequestAuth(ctx context.Context) (oauth.Authorization, error) {
	credential, err := a.token(ctx)
	if err != nil {
		return oauth.Authorization{}, err
	}
	return oauth.Authorization{Token: credential.Access, AccountID: credential.AccountID}, nil
}
