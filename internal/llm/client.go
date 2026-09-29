package llm

import (
	"context"
	"fmt"
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
		provider, ok := state.config.Providers[definition.Provider]
		if !ok || provider.APIKey == "" || !validProtocol(protocolFor(definition.Provider, provider)) {
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

// ValidateConfig 检查模型对应的本地 Provider 是否配置了密钥。
func (c *Client) ValidateConfig(id string) error {
	state := c.state()
	definition, ok := state.models[id]
	if !ok || state.config.Providers[definition.Provider].APIKey == "" {
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
}

type modelState struct {
	config config
	models map[string]model
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
	pinned := &Client{}
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
	providerConfig, ok := state.config.Providers[definition.Provider]
	if !ok {
		return nil, fmt.Errorf("llm: provider %q is not configured", definition.Provider)
	}
	if providerConfig.APIKey == "" {
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
	if protocolFor(definition.Provider, providerConfig) == "openai-chat" {
		copyOptions := make(map[string]any, len(options)+1)
		for key, value := range options {
			copyOptions[key] = value
		}
		copyOptions["useResponsesAPI"] = false
		options = copyOptions
	}
	model, err := newModel(definition, providerConfig)
	if err != nil {
		return nil, err
	}
	toolDefinitions := toProviderTools(input.Tools)
	params := provider.GenerateParams{
		System:          input.System,
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
	client := &Client{files: files}
	client.current.Store(&modelState{config: cfg, models: models})
	return client, nil
}
