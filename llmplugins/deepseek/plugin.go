// Package deepseek 通过官方 OpenAI Go SDK 接入 DeepSeek Chat Completions。
package deepseek

import (
	"fmt"

	"harness/core"
	"harness/llm"
)

// Plugin 组合 DeepSeek 的密钥和地址，启动时把适配器登记到 llm。
type Plugin struct {
	APIKey  string // 用户配置里的 DeepSeek 密钥，不从环境变量读取
	BaseURL string // DeepSeek API 根地址
}

// Name 返回插件名。
func (Plugin) Name() string {
	return "llm-deepseek"
}

// Start 领取 llm 并登记名为 deepseek 的适配器。
func (p Plugin) Start(app *core.App) error {
	service, err := llm.Get(app)
	if err != nil {
		return fmt.Errorf("deepseek 缺少 llm：%w", err)
	}
	if p.APIKey == "" {
		return fmt.Errorf("DeepSeek API key 不能为空")
	}
	if p.BaseURL == "" {
		return fmt.Errorf("DeepSeek base URL 不能为空")
	}
	adapter := newAdapter(p.APIKey, p.BaseURL)
	err = service.Register(adapter)
	if err != nil {
		return fmt.Errorf("登记 DeepSeek 适配器失败：%w", err)
	}
	return nil
}
