package llm

import (
	"context"
	"fmt"
	"sort"

	"github.com/zendev-sh/goai/provider"
)

// Models 返回当前已配置 Provider 可用的模型、窗口、是否看图和思考档位。
func (c *Client) Models() []ModelChoice {
	out := make([]ModelChoice, 0, len(c.models))
	for id, definition := range c.models {
		if _, ok := c.config.Providers[definition.Provider]; !ok {
			continue
		}
		efforts := make([]string, 0, len(definition.Reasoning))
		for effort := range definition.Reasoning {
			efforts = append(efforts, effort)
		}
		sort.Strings(efforts)
		out = append(out, ModelChoice{
			ID:               id,
			ContextWindow:    definition.ContextWindow,
			Vision:           definition.Vision,
			ReasoningEfforts: efforts,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ContextWindow 返回指定模型的窗口大小；未知模型返回 0。
func (c *Client) ContextWindow(id string) int {
	if c == nil {
		return 0
	}
	definition, ok := c.models[id]
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
	definition, ok := c.models[id]
	return ok && definition.Vision
}

// 活对象。用启动时读取的本机配置和模型定义发起模型调用。
type Client struct {
	config config
	models map[string]model
}

func newClient(cfg config) (*Client, error) {
	models, err := loadModels()
	if err != nil {
		return nil, err
	}
	return &Client{config: cfg, models: models}, nil
}

// Stream 根据本次调用配置选择模型和思考档位，直接返回 goai 的流事件。
func (c *Client) Stream(ctx context.Context, config RunConfig, input Input) (<-chan provider.StreamChunk, error) {
	definition, ok := c.models[config.Model]
	if !ok {
		return nil, fmt.Errorf("llm: unknown model %q", config.Model)
	}
	providerConfig, ok := c.config.Providers[definition.Provider]
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
	model, err := newModel(definition, providerConfig)
	if err != nil {
		return nil, err
	}
	toolDefinitions := toProviderTools(input.Tools)
	stream, err := model.DoStream(ctx, provider.GenerateParams{
		System:          input.System,
		Messages:        messages,
		Tools:           toolDefinitions,
		ProviderOptions: options,
	})
	if err != nil {
		return nil, err
	}
	return stream.Stream, nil
}
