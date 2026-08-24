// Package compose 组装 dsh 所需的持久化、模型、工具和循环插件。
package compose

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"harness/agents"
	"harness/core"
	"harness/llm"
	"harness/llmplugins/deepseek"
	"harness/localenv"
	"harness/loop"
	"harness/persistence/jsonl"
	"harness/persistence/profilejson"
	"harness/session"
	filetools "harness/toolplugins/files"
	"harness/tools"
)

const defaultDeepSeekBaseURL = "https://api.deepseek.com"

// Config 是 ~/.harness/config.json 的全部配置，不放工作目录和会话状态。
type Config struct {
	Version   int             `json:"version"`
	Providers ProviderConfigs `json:"providers"`
}

// ProviderConfigs 放按服务商划分的连接配置。
type ProviderConfigs struct {
	DeepSeek DeepSeekConfig `json:"deepseek"`
}

// DeepSeekConfig 是 DeepSeek 连接所需的用户配置。
type DeepSeekConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

// Runtime 是一次已经组装好的 dsh 运行时。
type Runtime struct {
	App       *core.App // 已按依赖顺序装好的公共场地
	Home      string    // 用户私有目录
	Workspace string    // 调用方明确给出的工作目录
}

// Open 读取用户配置并组装完整运行时。首次调用会写无密钥模板后返回错误。
func Open(home string, workspace string) (*Runtime, error) {
	absoluteHome, err := privateDirectory(home, "用户目录")
	if err != nil {
		return nil, err
	}
	absoluteWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录失败：%w", err)
	}
	info, err := os.Stat(absoluteWorkspace)
	if err != nil {
		return nil, fmt.Errorf("打开工作目录失败：%w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("工作目录 %s 不是目录", absoluteWorkspace)
	}

	config, err := loadConfig(absoluteHome)
	if err != nil {
		return nil, err
	}
	app := core.New()
	err = app.Install(
		profilejson.Plugin{Root: filepath.Join(absoluteHome, "agents")},
		jsonl.Plugin{Root: filepath.Join(absoluteHome, "sessions")},
		session.Plugin{},
		llm.Plugin{},
		deepseek.Plugin{APIKey: config.Providers.DeepSeek.APIKey, BaseURL: config.Providers.DeepSeek.BaseURL},
		tools.Plugin{},
		localenv.Plugin{Root: absoluteWorkspace},
		filetools.Plugin{},
		agents.Plugin{},
		loop.Plugin{},
	)
	if err != nil {
		app.Close()
		return nil, err
	}
	return &Runtime{App: app, Home: absoluteHome, Workspace: absoluteWorkspace}, nil
}

func privateDirectory(path string, label string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%s不能为空", label)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析%s失败：%w", label, err)
	}
	err = os.MkdirAll(absolute, 0o700)
	if err != nil {
		return "", fmt.Errorf("创建%s失败：%w", label, err)
	}
	err = os.Chmod(absolute, 0o700)
	if err != nil {
		return "", fmt.Errorf("收紧%s权限失败：%w", label, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("打开%s失败：%w", label, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s路径 %s 不是目录", label, absolute)
	}
	return absolute, nil
}

func loadConfig(home string) (Config, error) {
	path := filepath.Join(home, "config.json")
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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config Config
	err = decoder.Decode(&config)
	if err != nil {
		return Config{}, fmt.Errorf("配置不是合法 JSON：%w", err)
	}
	err = expectEOF(decoder)
	if err != nil {
		return Config{}, err
	}
	if config.Version != 1 {
		return Config{}, fmt.Errorf("配置版本必须是 1")
	}
	if config.Providers.DeepSeek.BaseURL == "" {
		config.Providers.DeepSeek.BaseURL = defaultDeepSeekBaseURL
	}
	if config.Providers.DeepSeek.APIKey == "" {
		return Config{}, fmt.Errorf("配置缺少 providers.deepseek.api_key")
	}
	return config, nil
}

func writeTemplate(path string) error {
	config := Config{
		Version: 1,
		Providers: ProviderConfigs{DeepSeek: DeepSeekConfig{
			BaseURL: defaultDeepSeekBaseURL,
		}},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("编码配置模板失败：%w", err)
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("创建配置模板失败：%w", err)
	}
	_, err = file.Write(data)
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

func expectEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("配置不是合法 JSON：%w", err)
	}
	return fmt.Errorf("配置只能有一个 JSON 对象")
}
