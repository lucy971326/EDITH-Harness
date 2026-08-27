// Package deepseek 通过官方 OpenAI Go SDK 接入 DeepSeek Chat Completions。
package deepseek

import (
	"fmt"
	"strings"

	"harness/config"
	"harness/core"
	"harness/llm"
)

const defaultBaseURL = "https://api.deepseek.com"

// Plugin 启动时领取自己的抽屉和钥匙，再把适配器登记到 llm。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "llm-deepseek"
}

// Start 领取 llm、settings、credentials，登记抽屉后填充 Adapter 插槽。
func (Plugin) Start(app *core.App) error {
	service, err := llm.Get(app)
	if err != nil {
		return fmt.Errorf("deepseek 缺少 llm：%w", err)
	}
	settings, err := config.GetSettings(app)
	if err != nil {
		return fmt.Errorf("deepseek 缺少 settings：%w", err)
	}
	creds, err := config.GetCredentials(app)
	if err != nil {
		return fmt.Errorf("deepseek 缺少 credentials：%w", err)
	}
	unregisterDrawer, err := settings.Register(config.Drawer{
		Name:     "deepseek",
		Defaults: map[string]string{"base_url": defaultBaseURL},
		Validate: func(values map[string]string) error {
			if strings.TrimSpace(values["base_url"]) == "" {
				return fmt.Errorf("DeepSeek 网址不能为空")
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	app.OnCleanup(unregisterDrawer)
	unregisterNeed, err := creds.Register(config.Need{Drawer: "deepseek", Key: "api_key"})
	if err != nil {
		return err
	}
	app.OnCleanup(unregisterNeed)
	values, err := settings.Get("deepseek")
	if err != nil {
		return err
	}
	apiKey, err := creds.Resolve("deepseek", "api_key")
	if err != nil {
		return fmt.Errorf("缺少 DeepSeek 钥匙")
	}
	adapter := newAdapter(apiKey, values["base_url"])
	err = service.Register(adapter)
	if err != nil {
		return fmt.Errorf("登记 DeepSeek 适配器失败：%w", err)
	}
	return nil
}
