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

// 数据。models.json 的根对象。
type modelFile struct {
	Models map[string]model `json:"models"`
}

// 数据。一条模型定义。
type model struct {
	Provider  string                    `json:"provider"`
	ID        string                    `json:"id"`
	Reasoning map[string]map[string]any `json:"reasoning"`
}

func loadModels() (map[string]model, error) {
	var file modelFile
	err := json.Unmarshal(modelsJSON, &file)
	if err != nil {
		return nil, fmt.Errorf("llm: parse models.json: %w", err)
	}
	return file.Models, nil
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
