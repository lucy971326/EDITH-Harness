package codex

import (
	"context"

	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/openai"
)

// Model 使用当前订阅凭据创建固定地址的 Codex Responses 模型。
func (a *Auth) Model(ctx context.Context, id string) (provider.LanguageModel, error) {
	credential, err := a.token(ctx)
	if err != nil {
		return nil, err
	}
	return openai.Chat(id,
		openai.WithAPIKey(credential.Access),
		openai.WithBaseURL(a.baseURL),
		openai.WithHeaders(map[string]string{
			"chatgpt-account-id": credential.AccountID,
			"originator":         "edith",
			"OpenAI-Beta":        "responses=experimental",
		})), nil
}

// Options 固定订阅通道需要的请求选项，不修改模型目录中的原始设置。
func Options(reasoning map[string]any) map[string]any {
	options := make(map[string]any, len(reasoning)+5)
	for key, value := range reasoning {
		options[key] = value
	}
	options["store"] = false
	options["include"] = []string{"reasoning.encrypted_content"}
	options["parallelToolCalls"] = true
	options["text_verbosity"] = "low"
	options["reasoning_summary"] = "auto"
	return options
}
