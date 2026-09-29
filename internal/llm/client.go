package llm

import (
	"context"
	"fmt"
	"harness/internal/llm/codex"
	"harness/internal/persist"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/zendev-sh/goai/provider"
)

// Models 返回当前已配置 Provider 可用的模型、窗口、是否看图和思考档位。
func (c *Client) Models() []ModelChoice {
	state := c.state()
	out := make([]ModelChoice, 0, len(state.models))
	for id, definition := range state.models {
		provider, ok := state.provider(definition.Provider)
		if !ok || !validProtocol(protocolFor(definition.Provider, provider)) || !c.providerReady(definition.Provider, provider) {
			continue
		}
		efforts := make([]string, 0, len(definition.Reasoning))
		for _, level := range definition.Reasoning {
			efforts = append(efforts, level.Effort)
		}
		out = append(out, ModelChoice{
			ID:               id,
			Provider:         definition.Provider,
			ContextWindow:    definition.ContextWindow,
			Vision:           definition.Vision,
			ReasoningEfforts: efforts,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ValidateConfig 检查模型对应的本地 Provider 是否具备可用凭据。
func (c *Client) ValidateConfig(id string) error {
	state := c.state()
	definition, ok := state.models[id]
	if !ok {
		return fmt.Errorf("llm: model provider is not configured")
	}
	providerConfig, configured := state.provider(definition.Provider)
	if !configured || !c.providerReady(definition.Provider, providerConfig) {
		return fmt.Errorf("llm: model provider is not configured")
	}
	return nil
}

// ContextWindow 返回指定模型的窗口大小；未知模型返回 0。
func (c *Client) ContextWindow(id string) int {
	if c == nil {
		return 0
	}
	definition, ok := c.state().models[id]
	if !ok {
		return 0
	}
	return definition.ContextWindow
}

// Vision 返回指定模型是否能看图；未知模型返回 false。
func (c *Client) Vision(id string) bool {
	if c == nil {
		return false
	}
	definition, ok := c.state().models[id]
	return ok && definition.Vision
}

// 活对象。当前目录原子替换；已开始的模型循环通过 Pin 保留旧目录。
type Client struct {
	// current 为完整不可变目录；编辑串行化，已开始的 Run 持有旧目录。
	current atomic.Pointer[modelState]
	editMu  sync.Mutex
	files   *persist.Files
	auth    *codex.Auth
}

type modelState struct {
	config config
	models map[string]model
}

func (s *modelState) provider(id string) (providerConfig, bool) {
	if id == "openai-codex" {
		return providerConfig{Protocol: "openai-codex"}, true
	}
	entry, ok := s.config.Providers[id]
	return entry, ok
}

func (c *Client) state() *modelState {
	if c == nil {
		return &modelState{}
	}
	if current := c.current.Load(); current != nil {
		return current
	}
	return &modelState{}
}

// Pin 固定一轮运行使用的目录，避免多步工具调用期间切换供应商设置。
func (c *Client) Pin() *Client {
	state := c.state()
	pinned := &Client{auth: c.auth}
	pinned.current.Store(state)
	return pinned
}

// Stream 根据本次调用配置选择模型和思考档位，直接返回 goai 的流事件。
func (c *Client) Stream(ctx context.Context, config RunConfig, input Input) (<-chan provider.StreamChunk, error) {
	state := c.state()
	definition, ok := state.models[config.Model]
	if !ok {
		return nil, fmt.Errorf("llm: unknown model %q", config.Model)
	}
	providerConfig, ok := state.provider(definition.Provider)
	if !ok {
		return nil, fmt.Errorf("llm: provider %q is not configured", definition.Provider)
	}
	protocol := protocolFor(definition.Provider, providerConfig)
	if protocol != "openai-codex" && providerConfig.APIKey == "" {
		return nil, fmt.Errorf("llm: provider %q has no API key", definition.Provider)
	}
	messages, err := toProviderMessages(input.History, definition.Vision)
	if err != nil {
		return nil, err
	}
	options, err := reasoningOptions(definition, config.ReasoningEffort)
	if err != nil {
		return nil, err
	}
	if protocol == "openai-chat" {
		copyOptions := make(map[string]any, len(options)+1)
		for key, value := range options {
			copyOptions[key] = value
		}
		copyOptions["useResponsesAPI"] = false
		options = copyOptions
	}
	if protocol == "openai-codex" {
		options = codex.Options(options)
	}
	var model provider.LanguageModel
	if protocol == "openai-codex" {
		if c.auth == nil {
			return nil, fmt.Errorf("llm: ChatGPT is not connected")
		}
		model, err = c.auth.Model(ctx, definition.ID)
	} else {
		model, err = newModel(definition, providerConfig)
	}
	if err != nil {
		return nil, err
	}
	toolDefinitions := toProviderTools(input.Tools)
	system := input.System
	if protocol == "openai-codex" && system == "" {
		system = "You are a helpful assistant."
	}
	params := provider.GenerateParams{
		System:          system,
		Messages:        messages,
		Tools:           toolDefinitions,
		ToolChoice:      input.ToolChoice,
		ProviderOptions: options,
	}
	stream, err := model.DoStream(ctx, params)
	if err != nil {
		return nil, err
	}
	return stream.Stream, nil
}

// New 从本机配置创建模型客户端。
func New(files *persist.Files) (*Client, error) {
	cfg, err := loadConfig(files)
	if err != nil {
		return nil, err
	}
	models, _, err := loadModelsFile(files)
	if err != nil {
		return nil, err
	}
	auth, err := codex.New(files)
	if err != nil {
		return nil, err
	}
	client := &Client{files: files, auth: auth}
	client.current.Store(&modelState{config: cfg, models: models})
	return client, nil
}

func (c *Client) providerReady(id string, config providerConfig) bool {
	if protocolFor(id, config) == "openai-codex" {
		return c.auth != nil && c.auth.Authenticated()
	}
	return config.APIKey != ""
}

// Close 取消尚未完成的浏览器授权并释放回调监听器。
func (c *Client) Close() error {
	if c.auth != nil {
		c.auth.Close()
	}
	return nil
}

// StartCodexAuth 开始本机浏览器登录并返回授权网址。
func (c *Client) StartCodexAuth() (codex.View, error) { return c.auth.Start() }

func (c *Client) CodexAuthStatus() codex.View          { return c.auth.Status() }
func (c *Client) CancelCodexAuth() codex.View          { return c.auth.Cancel() }
func (c *Client) LogoutCodexAuth() (codex.View, error) { return c.auth.Logout() }
