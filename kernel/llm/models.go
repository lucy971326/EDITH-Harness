package llm

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/deepseek"
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

func newClient(config config) (*Client, error) {
	var file modelFile
	if err := json.Unmarshal(modelsJSON, &file); err != nil {
		return nil, fmt.Errorf("llm: parse models.json: %w", err)
	}
	return &Client{config: config, models: file.Models}, nil
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

func newModel(definition model, config providerConfig) (provider.LanguageModel, error) {
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
