package llm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/deepseek"

	"harness/kernel/kinds"
	"harness/kernel/session"
)

//go:embed models.json
var modelsJSON []byte

type modelFile struct {
	Models map[string]model `json:"models"`
}

type model struct {
	Provider  string                    `json:"provider"`
	ID        string                    `json:"id"`
	Reasoning map[string]map[string]any `json:"reasoning"`
}

// Input 是 Runner 已准备好的本轮输入。
type Input struct {
	System  string
	History []session.Message
	Tools   []provider.ToolDefinition
}

// Client 用本机 Config 和内置模型定义发起一次模型调用。
type Client struct {
	config Config
	models map[string]model
}

func NewClient(config Config) (*Client, error) {
	var file modelFile
	if err := json.Unmarshal(modelsJSON, &file); err != nil {
		return nil, fmt.Errorf("llm: parse models.json: %w", err)
	}
	return &Client{config: config, models: file.Models}, nil
}

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

func reasoningOptions(definition model, effort string) (map[string]any, error) {
	if effort == "" {
		return nil, nil
	}
	options, ok := definition.Reasoning[effort]
	if !ok {
		return nil, fmt.Errorf("llm: model %q does not support reasoning effort %q", definition.ID, effort)
	}
	return options, nil
}

func newModel(definition model, config ProviderConfig) (provider.LanguageModel, error) {
	switch definition.Provider {
	case "deepseek":
		options := []deepseek.Option{deepseek.WithAPIKey(config.APIKey)}
		if config.BaseURL != "" {
			options = append(options, deepseek.WithBaseURL(config.BaseURL))
		}
		return deepseek.Chat(definition.ID, options...), nil
	default:
		return nil, fmt.Errorf("llm: provider %q is not supported", definition.Provider)
	}
}
