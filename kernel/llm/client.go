package llm

import (
	"context"
	"fmt"

	"github.com/zendev-sh/goai/provider"

	"harness/kernel/kinds"
)

// Stream 根据 Setup 选择模型和思考档位，直接返回 goai 的流事件。
func (c *Client) Stream(ctx context.Context, setup kinds.Setup, input Input) (<-chan provider.StreamChunk, error) {
	definition, ok := c.models[setup.Model]
	if !ok {
		return nil, fmt.Errorf("llm: unknown model %q", setup.Model)
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
	options, err := reasoningOptions(definition, setup.ReasoningEffort)
	if err != nil {
		return nil, err
	}
	model, err := newModel(definition, providerConfig)
	if err != nil {
		return nil, err
	}
	stream, err := model.DoStream(ctx, provider.GenerateParams{
		System:          input.System,
		Messages:        messages,
		Tools:           input.Tools,
		ProviderOptions: options,
	})
	if err != nil {
		return nil, err
	}
	return stream.Stream, nil
}
