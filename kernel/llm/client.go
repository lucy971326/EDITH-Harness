package llm

import (
	"context"
	"fmt"

	"github.com/zendev-sh/goai/provider"
)

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

	messages, err := toProviderMessages(input.History)
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
