package llm

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"harness/internal/persist"

	"gopkg.in/yaml.v3"
)

// 数据。本机部署配置。API key 只放本地 YAML，不进 models.json。
type config struct {
	Providers map[string]providerConfig `yaml:"providers"`
}

// 数据。一家 Provider 的请求协议、本机密钥和地址。
type providerConfig struct {
	Protocol string `yaml:"protocol,omitempty"`
	APIKey   string `yaml:"apiKey"`
	BaseURL  string `yaml:"baseURL"`
}

func loadConfig(files *persist.Files) (config, error) {
	body, err := files.Read("config.yaml")
	if errors.Is(err, os.ErrNotExist) {
		return config{Providers: map[string]providerConfig{}}, nil
	}
	if err != nil {
		return config{}, fmt.Errorf("llm: read config: %w", err)
	}
	return parseConfig(body)
}

func parseConfig(body []byte) (config, error) {
	var out config
	if err := yaml.Unmarshal(body, &out); err != nil {
		return config{}, fmt.Errorf("llm: parse config: %w", err)
	}
	return out, nil
}

func providerURL(id string, config providerConfig) string {
	if config.BaseURL != "" {
		return config.BaseURL
	}
	if preset, ok := catalog.Providers[id]; ok {
		return preset.BaseURL
	}
	switch protocolFor(id, config) {
	case "openai-chat", "openai-responses":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com"
	}
	return ""
}

// 混合网关的 API 路径属于接入协议，用户只配置供应商根地址。
func modelURL(definition model, config providerConfig) string {
	base := strings.TrimRight(providerURL(definition.Provider, config), "/")
	protocol := modelProtocol(definition, config)
	switch definition.Provider {
	case "opencode", "opencode-go", "fireworks":
		if protocol == "anthropic" {
			return strings.TrimSuffix(base, "/v1")
		}
	case "cloudflare-ai-gateway":
		switch protocol {
		case "anthropic":
			return base + "/anthropic"
		case "openai-responses":
			return base + "/openai"
		default:
			return base + "/compat"
		}
	}
	return base
}
