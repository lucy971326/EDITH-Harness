package llm

import (
	"errors"
	"fmt"
	"os"

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
