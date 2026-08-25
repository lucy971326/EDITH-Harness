package compose

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const defaultDeepSeekBaseURL = "https://api.deepseek.com"

const configTemplate = `version: 2

plugins:
  project_store: projectjson
  preset_store: presetjson
  journal: jsonl
  llm_adapters:
    - deepseek
  environment: localenv
  tool_plugins:
    - files
  runner: loop
  ui: web

providers:
  deepseek:
    api_key: ""
    base_url: https://api.deepseek.com
`

// Config 是 ~/.harness/config.yaml 的全部配置，不放工作目录和会话状态。
type Config struct {
	Version   int             `yaml:"version"`
	Plugins   PluginConfig    `yaml:"plugins"`
	Providers ProviderConfigs `yaml:"providers"`
}

// PluginConfig 记录本次启动选择的可替换大零件。
type PluginConfig struct {
	ProjectStore string   `yaml:"project_store"`
	PresetStore  string   `yaml:"preset_store"`
	Journal      string   `yaml:"journal"`
	LLMAdapters  []string `yaml:"llm_adapters"`
	Environment  string   `yaml:"environment"`
	ToolPlugins  []string `yaml:"tool_plugins"`
	Runner       string   `yaml:"runner"`
	UI           string   `yaml:"ui"`
}

// ProviderConfigs 放按服务商划分的连接配置。
type ProviderConfigs struct {
	DeepSeek DeepSeekConfig `yaml:"deepseek"`
}

// DeepSeekConfig 是 DeepSeek 连接所需的用户配置。
type DeepSeekConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

func loadConfig(home string) (Config, error) {
	path := filepath.Join(home, "config.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		err = writeTemplate(path)
		if err != nil {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("已创建 %s；请填入 providers.deepseek.api_key 后重新运行", path)
	}
	if err != nil {
		return Config{}, fmt.Errorf("读取配置失败：%w", err)
	}
	err = os.Chmod(path, 0o600)
	if err != nil {
		return Config{}, fmt.Errorf("收紧配置文件权限失败：%w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var config Config
	err = decoder.Decode(&config)
	if err != nil {
		return Config{}, fmt.Errorf("配置不是合法 YAML：%w", err)
	}
	err = expectYAMLEOF(decoder)
	if err != nil {
		return Config{}, err
	}
	if config.Plugins.UI == "" {
		config.Plugins.UI = "web"
	}
	err = validateConfig(config)
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

// DumpConfig 读取并校验配置后写成标准 YAML，供查看或再次导入。
func DumpConfig(home string, output io.Writer) error {
	absoluteHome, err := privateDirectory(home, "用户目录")
	if err != nil {
		return err
	}
	config, err := loadConfig(absoluteHome)
	if err != nil {
		return err
	}
	_, err = selectPlugins(config, absoluteHome)
	if err != nil {
		return fmt.Errorf("配置不能组装：%w", err)
	}
	encoder := yaml.NewEncoder(output)
	encoder.SetIndent(2)
	err = encoder.Encode(config)
	if err == nil {
		err = encoder.Close()
	}
	if err != nil {
		return fmt.Errorf("导出配置失败：%w", err)
	}
	return nil
}

func validateConfig(config Config) error {
	if config.Version != 2 {
		return fmt.Errorf("配置版本必须是 2")
	}
	if config.Plugins.ProjectStore == "" {
		return fmt.Errorf("配置缺少 plugins.project_store")
	}
	if config.Plugins.PresetStore == "" {
		return fmt.Errorf("配置缺少 plugins.preset_store")
	}
	if config.Plugins.Journal == "" {
		return fmt.Errorf("配置缺少 plugins.journal")
	}
	if len(config.Plugins.LLMAdapters) == 0 {
		return fmt.Errorf("配置至少需要一个 plugins.llm_adapters")
	}
	if config.Plugins.Environment == "" {
		return fmt.Errorf("配置缺少 plugins.environment")
	}
	if config.Plugins.ToolPlugins == nil {
		return fmt.Errorf("配置缺少 plugins.tool_plugins；不装工具时请写 []")
	}
	if config.Plugins.Runner == "" {
		return fmt.Errorf("配置缺少 plugins.runner")
	}
	if config.Plugins.UI == "" {
		return fmt.Errorf("配置缺少 plugins.ui")
	}
	err := rejectDuplicates("plugins.llm_adapters", config.Plugins.LLMAdapters)
	if err != nil {
		return err
	}
	err = rejectDuplicates("plugins.tool_plugins", config.Plugins.ToolPlugins)
	if err != nil {
		return err
	}
	return nil
}

func rejectDuplicates(field string, names []string) error {
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if name == "" {
			return fmt.Errorf("配置 %s 不能包含空名称", field)
		}
		if seen[name] {
			return fmt.Errorf("配置 %s 重复选择了 %s", field, name)
		}
		seen[name] = true
	}
	return nil
}

func writeTemplate(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("创建配置模板失败：%w", err)
	}
	_, err = file.WriteString(configTemplate)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		removeErr := os.Remove(path)
		return errors.Join(fmt.Errorf("写配置模板失败：%w", err), removeErr)
	}
	return nil
}

func expectYAMLEOF(decoder *yaml.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("配置不是合法 YAML：%w", err)
	}
	return fmt.Errorf("配置只能有一份 YAML 文档")
}
