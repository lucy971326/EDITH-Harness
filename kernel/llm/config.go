package llm

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config 是本机部署配置。API key 只放本地 YAML，不进 models.json。
type Config struct {
	Providers map[string]ProviderConfig `yaml:"providers"`
}

type ProviderConfig struct {
	APIKey  string `yaml:"apiKey"`
	BaseURL string `yaml:"baseURL"`
}

// LoadConfig 从 ~/.harness/config.yaml 读取 Provider 地址和 API key。
func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("llm: find home: %w", err)
	}
	return loadConfig(filepath.Join(home, ".harness", "config.yaml"))
}

func loadConfig(path string) (Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("llm: read config: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(body, &config); err != nil {
		return Config{}, fmt.Errorf("llm: parse config: %w", err)
	}
	return config, nil
}
