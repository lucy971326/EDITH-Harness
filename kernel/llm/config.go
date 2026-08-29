package llm

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// config 是本机部署配置。API key 只放本地 YAML，不进 models.json。
type config struct {
	Providers map[string]providerConfig `yaml:"providers"`
}

type providerConfig struct {
	APIKey  string `yaml:"apiKey"`
	BaseURL string `yaml:"baseURL"`
}

func loadConfig() (config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config{}, fmt.Errorf("llm: find home: %w", err)
	}
	return readConfig(filepath.Join(home, ".harness", "config.yaml"))
}

func readConfig(path string) (config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("llm: read config: %w", err)
	}

	var out config
	if err := yaml.Unmarshal(body, &out); err != nil {
		return config{}, fmt.Errorf("llm: parse config: %w", err)
	}
	return out, nil
}
