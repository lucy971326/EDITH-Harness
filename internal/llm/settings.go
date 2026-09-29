package llm

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrInvalidSettings = errors.New("llm: invalid settings")
	ErrSettingsChanged = errors.New("llm: settings changed")
	ErrSettingsMissing = errors.New("llm: settings item missing")
)

func revision(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }

func (c *Client) readDiskLocked() (config, map[string]model, []byte, []byte, error) {
	configBody, err := c.files.Read("config.yaml")
	if errors.Is(err, os.ErrNotExist) {
		configBody = nil
		err = nil
	}
	if err != nil {
		return config{}, nil, nil, nil, err
	}
	cfg, err := parseConfig(configBody)
	if err != nil {
		return config{}, nil, nil, nil, err
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]providerConfig{}
	}
	modelBody, err := c.files.Read("models.json")
	if errors.Is(err, os.ErrNotExist) {
		return cfg, map[string]model{}, configBody, nil, nil
	}
	if err != nil {
		return config{}, nil, nil, nil, err
	}
	models, err := parseModels(modelBody)
	return cfg, models, configBody, modelBody, err
}

func (c *Client) reloadLocked() (SettingsView, error) {
	cfg, models, configBody, modelBody, err := c.readDiskLocked()
	if err != nil {
		return SettingsView{}, err
	}
	c.current.Store(&modelState{config: cfg, models: models})
	return settingsView(cfg, models, configBody, modelBody), nil
}

// ReadSettings 返回供表单编辑的目录和版本，不返回 API 密钥。
func (c *Client) ReadSettings() (SettingsView, error) {
	c.editMu.Lock()
	defer c.editMu.Unlock()
	return c.reloadLocked()
}

func settingsView(cfg config, models map[string]model, configBody, modelBody []byte) SettingsView {
	view := SettingsView{Providers: []ProviderSettings{}, Models: []ModelSettings{}, Presets: []ModelSettings{},
		ProviderRevision: revision(configBody), ModelRevision: revision(modelBody)}
	ids := map[string]bool{"openai-codex": true}
	for id := range cfg.Providers {
		ids[id] = true
	}
	for _, item := range models {
		ids[item.Provider] = true
	}
	for id := range ids {
		entry := cfg.Providers[id]
		view.Providers = append(view.Providers, ProviderSettings{
			ID: id, Protocol: protocolFor(id, entry), BaseURL: entry.BaseURL, HasAPIKey: entry.APIKey != "",
		})
	}
	sort.Slice(view.Providers, func(i, j int) bool { return view.Providers[i].ID < view.Providers[j].ID })
	for key, item := range models {
		entry := cfg.Providers[item.Provider]
		view.Models = append(view.Models, modelSettings(key, item, protocolFor(item.Provider, entry)))
	}
	sort.Slice(view.Models, func(i, j int) bool { return view.Models[i].Key < view.Models[j].Key })
	presets, err := loadModels()
	if err == nil {
		for key, item := range presets {
			view.Presets = append(view.Presets, modelSettings(key, item, item.Provider))
		}
		sort.Slice(view.Presets, func(i, j int) bool { return view.Presets[i].Key < view.Presets[j].Key })
	}
	return view
}

func modelSettings(key string, item model, protocol string) ModelSettings {
	setting := ModelSettings{Key: key, Provider: item.Provider, ID: item.ID,
		ContextWindow: item.ContextWindow, Vision: item.Vision, Reasoning: []ReasoningSettings{}}
	for _, level := range item.Reasoning {
		setting.Reasoning = append(setting.Reasoning, levelSettings(level, protocol))
	}
	return setting
}

func levelSettings(level reasoningLevel, protocol string) ReasoningSettings {
	result := ReasoningSettings{Name: level.Effort, Mode: "enabled"}
	if level.Effort == "off" {
		result.Mode = "off"
	}
	switch protocol {
	case "deepseek":
		result.Effort, _ = level.Options["reasoning_effort"].(string)
	case "google":
		google, _ := level.Options["google"].(map[string]any)
		thinking, _ := google["thinkingConfig"].(map[string]any)
		result.Effort, _ = thinking["thinkingLevel"].(string)
	case "openai-chat", "openai-responses", "openai-codex":
		result.Effort, _ = level.Options["reasoning_effort"].(string)
	case "anthropic":
		thinking, _ := level.Options["thinking"].(map[string]any)
		if kind, _ := thinking["type"].(string); kind == "adaptive" {
			result.Mode = "adaptive"
		}
		if budget, ok := thinking["budgetTokens"].(float64); ok {
			result.BudgetTokens = int(budget)
		}
		result.Effort, _ = level.Options["effort"].(string)
	}
	return result
}

func validProviderID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, letter := range id {
		if letter >= 'a' && letter <= 'z' || letter >= '0' && letter <= '9' || letter == '-' || letter == '_' {
			continue
		}
		return false
	}
	return true
}

func validateProvider(input SaveProviderInput) error {
	if !validProviderID(input.ID) || !validProtocol(input.Protocol) || input.ClearAPIKey && input.APIKey != "" {
		return fmt.Errorf("%w: 供应商 ID、协议或密钥操作无效", ErrInvalidSettings)
	}
	if input.ID == "openai-codex" || input.Protocol == "openai-codex" {
		return fmt.Errorf("%w: ChatGPT 订阅连接由内置登录管理", ErrInvalidSettings)
	}
	if strings.TrimSpace(input.BaseURL) != "" {
		parsed, err := url.Parse(input.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return fmt.Errorf("%w: API 地址必须是 http(s) URL", ErrInvalidSettings)
		}
	}
	return nil
}

func encodeProviders(body []byte, providers map[string]providerConfig) ([]byte, error) {
	var document yaml.Node
	if len(strings.TrimSpace(string(body))) == 0 {
		document = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	} else if err := yaml.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config.yaml must contain a mapping")
	}
	var encoded yaml.Node
	if err := encoded.Encode(providers); err != nil {
		return nil, err
	}
	root := document.Content[0]
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value == "providers" {
			root.Content[index+1] = &encoded
			return yaml.Marshal(&document)
		}
	}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "providers"}, &encoded)
	return yaml.Marshal(&document)
}

// SaveProvider 原子替换本机配置里的 providers 段，保留 Jev 和其他段。
func (c *Client) SaveProvider(input SaveProviderInput) (SettingsView, error) {
	if err := validateProvider(input); err != nil {
		return SettingsView{}, err
	}
	c.editMu.Lock()
	defer c.editMu.Unlock()
	cfg, models, configBody, _, err := c.readDiskLocked()
	if err != nil {
		return SettingsView{}, err
	}
	if revision(configBody) != input.Revision {
		return SettingsView{}, ErrSettingsChanged
	}
	if existing, ok := cfg.Providers[input.ID]; ok && protocolFor(input.ID, existing) != input.Protocol {
		for _, item := range models {
			if item.Provider == input.ID {
				return SettingsView{}, fmt.Errorf("%w: 请先删除该供应商下的模型，再切换请求协议", ErrInvalidSettings)
			}
		}
	}
	old := cfg.Providers[input.ID]
	if input.APIKey != "" {
		old.APIKey = input.APIKey
	}
	if input.ClearAPIKey {
		old.APIKey = ""
	}
	old.Protocol = input.Protocol
	old.BaseURL = strings.TrimSpace(input.BaseURL)
	cfg.Providers[input.ID] = old
	body, err := encodeProviders(configBody, cfg.Providers)
	if err != nil {
		return SettingsView{}, err
	}
	if err := c.files.Write("config.yaml", body); err != nil {
		_, _ = c.reloadLocked()
		return SettingsView{}, err
	}
	return c.reloadLocked()
}

func encodeModels(models map[string]model) ([]byte, error) {
	body, err := json.MarshalIndent(modelFile{Models: models}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func modelFromSettings(input ModelSettings, protocol string) (string, model, error) {
	if !validProviderID(input.Provider) || strings.TrimSpace(input.ID) != input.ID || input.ID == "" || len(input.ID) > 200 ||
		strings.ContainsAny(input.ID, "\r\n\t") || input.ContextWindow <= 0 || input.ContextWindow > 100000000 || len(input.Reasoning) == 0 {
		return "", model{}, fmt.Errorf("%w: 模型 ID、窗口或档位无效", ErrInvalidSettings)
	}
	key := input.Provider + "/" + input.ID
	item := model{Provider: input.Provider, ID: input.ID, ContextWindow: input.ContextWindow,
		Vision: input.Vision, Reasoning: make(reasoningLevels, 0, len(input.Reasoning))}
	seen := map[string]bool{}
	for _, value := range input.Reasoning {
		if value.Name == "" || len(value.Name) > 32 || seen[value.Name] ||
			(value.Name == "off") != (value.Mode == "off") || len(value.Effort) > 40 {
			return "", model{}, fmt.Errorf("%w: 思考档位名称或模式无效", ErrInvalidSettings)
		}
		seen[value.Name] = true
		options, err := levelOptions(value, protocol)
		if err != nil {
			return "", model{}, err
		}
		item.Reasoning = append(item.Reasoning, reasoningLevel{Effort: value.Name, Options: options})
	}
	return key, item, nil
}

func levelOptions(value ReasoningSettings, protocol string) (map[string]any, error) {
	if value.Mode != "off" && value.Mode != "enabled" && !(protocol == "anthropic" && value.Mode == "adaptive") {
		return nil, fmt.Errorf("%w: 未知思考模式", ErrInvalidSettings)
	}
	if value.Mode != "off" && value.Effort == "" {
		return nil, fmt.Errorf("%w: 思考强度不能为空", ErrInvalidSettings)
	}
	switch protocol {
	case "deepseek":
		if value.Mode == "off" {
			return map[string]any{"thinking": map[string]any{"type": "disabled"}}, nil
		}
		return map[string]any{"thinking": map[string]any{"type": "enabled"}, "reasoning_effort": value.Effort}, nil
	case "google":
		if value.Mode == "off" {
			return map[string]any{"google": map[string]any{"thinkingConfig": false}}, nil
		}
		level := value.Effort
		return map[string]any{"google": map[string]any{"thinkingConfig": map[string]any{
			"thinkingLevel": level, "includeThoughts": true,
		}}}, nil
	case "openai-chat", "openai-responses", "openai-codex":
		if value.Mode == "off" {
			return map[string]any{"reasoning_effort": "none"}, nil
		}
		return map[string]any{"reasoning_effort": value.Effort}, nil
	case "anthropic":
		if value.Mode == "off" {
			return map[string]any{}, nil
		}
		thinking := map[string]any{"type": value.Mode}
		if value.Mode == "enabled" {
			if value.BudgetTokens < 1024 {
				return nil, fmt.Errorf("%w: 思考预算至少 1024 token", ErrInvalidSettings)
			}
			thinking["budgetTokens"] = value.BudgetTokens
		}
		return map[string]any{"thinking": thinking, "effort": value.Effort}, nil
	default:
		return nil, fmt.Errorf("%w: 不支持的供应商协议", ErrInvalidSettings)
	}
}

// SaveModel 保存一条模型定义，档位在 JSON 中保持用户排列顺序。
func (c *Client) SaveModel(input SaveModelInput) (SettingsView, error) {
	c.editMu.Lock()
	defer c.editMu.Unlock()
	cfg, models, _, modelBody, err := c.readDiskLocked()
	if err != nil {
		return SettingsView{}, err
	}
	if revision(modelBody) != input.Revision {
		return SettingsView{}, ErrSettingsChanged
	}
	provider, ok := cfg.Providers[input.Model.Provider]
	if input.Model.Provider == "openai-codex" {
		provider, ok = providerConfig{Protocol: "openai-codex"}, true
	}
	if !ok || !validProtocol(protocolFor(input.Model.Provider, provider)) {
		return SettingsView{}, fmt.Errorf("%w: 请先保存供应商", ErrInvalidSettings)
	}
	key, item, err := modelFromSettings(input.Model, protocolFor(input.Model.Provider, provider))
	if err != nil {
		return SettingsView{}, err
	}
	if input.Model.Key != "" && input.Model.Key != key {
		return SettingsView{}, fmt.Errorf("%w: 模型 ID 不可修改", ErrInvalidSettings)
	}
	if input.Model.Key == "" {
		if _, exists := models[key]; exists {
			return SettingsView{}, fmt.Errorf("%w: 模型已存在", ErrInvalidSettings)
		}
	} else if _, exists := models[key]; !exists {
		return SettingsView{}, ErrSettingsMissing
	}
	models[key] = item
	body, err := encodeModels(models)
	if err != nil {
		return SettingsView{}, err
	}
	if err := c.files.Write("models.json", body); err != nil {
		_, _ = c.reloadLocked()
		return SettingsView{}, err
	}
	return c.reloadLocked()
}

// DeleteModel 直接删除模型；会话中的旧 ID 保留，须在下次发送前重新选择。
func (c *Client) DeleteModel(key, expectedRevision string) (SettingsView, error) {
	c.editMu.Lock()
	defer c.editMu.Unlock()
	_, models, _, modelBody, err := c.readDiskLocked()
	if err != nil {
		return SettingsView{}, err
	}
	if revision(modelBody) != expectedRevision {
		return SettingsView{}, ErrSettingsChanged
	}
	if _, ok := models[key]; !ok {
		return SettingsView{}, ErrSettingsMissing
	}
	delete(models, key)
	body, err := encodeModels(models)
	if err != nil {
		return SettingsView{}, err
	}
	if err := c.files.Write("models.json", body); err != nil {
		_, _ = c.reloadLocked()
		return SettingsView{}, err
	}
	return c.reloadLocked()
}

// DeleteProvider 删除供应商及其模型；若第二次写入失败，按磁盘状态恢复当前目录。
func (c *Client) DeleteProvider(id, providerRevision, modelRevision string) (SettingsView, error) {
	if id == "openai-codex" {
		return SettingsView{}, fmt.Errorf("%w: ChatGPT 内置连接不可删除", ErrInvalidSettings)
	}
	c.editMu.Lock()
	defer c.editMu.Unlock()
	cfg, models, configBody, modelBody, err := c.readDiskLocked()
	if err != nil {
		return SettingsView{}, err
	}
	if revision(configBody) != providerRevision || revision(modelBody) != modelRevision {
		return SettingsView{}, ErrSettingsChanged
	}
	if _, ok := cfg.Providers[id]; !ok {
		found := false
		for _, item := range models {
			if item.Provider == id {
				found = true
				break
			}
		}
		if !found {
			return SettingsView{}, ErrSettingsMissing
		}
	}
	delete(cfg.Providers, id)
	for key, item := range models {
		if item.Provider == id {
			delete(models, key)
		}
	}
	newModels, err := encodeModels(models)
	if err != nil {
		return SettingsView{}, err
	}
	newConfig, err := encodeProviders(configBody, cfg.Providers)
	if err != nil {
		return SettingsView{}, err
	}
	if err := c.files.Write("models.json", newModels); err != nil {
		_, _ = c.reloadLocked()
		return SettingsView{}, err
	}
	if err := c.files.Write("config.yaml", newConfig); err != nil {
		_, _ = c.reloadLocked()
		return SettingsView{}, err
	}
	return c.reloadLocked()
}
