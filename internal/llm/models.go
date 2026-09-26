package llm

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/zendev-sh/goai/provider"
	"github.com/zendev-sh/goai/provider/deepseek"
	"github.com/zendev-sh/goai/provider/google"
)

//go:embed models.json
var modelsJSON []byte

// 数据。models.json 的根对象。
type modelFile struct {
	Models map[string]model `json:"models"`
}

// 数据。一条模型定义。
type model struct {
	Provider      string          `json:"provider"`
	ID            string          `json:"id"`
	ContextWindow int             `json:"contextWindow"`
	Vision        bool            `json:"vision"`
	Reasoning     reasoningLevels `json:"reasoning"`
}

// reasoningLevels 保留配置中的档位顺序，供界面从低到高展示。
type reasoningLevels []reasoningLevel

type reasoningLevel struct {
	Effort  string
	Options map[string]any
}

func (levels *reasoningLevels) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("reasoning must be an object")
	}
	var ordered reasoningLevels
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		effort := token.(string)
		for _, level := range ordered {
			if level.Effort == effort {
				return fmt.Errorf("duplicate reasoning effort %q", effort)
			}
		}
		var options map[string]any
		err = decoder.Decode(&options)
		if err != nil {
			return err
		}
		ordered = append(ordered, reasoningLevel{Effort: effort, Options: options})
	}
	_, err = decoder.Token()
	if err != nil {
		return err
	}
	*levels = ordered
	return nil
}

func loadModels() (map[string]model, error) {
	return parseModels(modelsJSON)
}

func parseModels(data []byte) (map[string]model, error) {
	var file modelFile
	err := json.Unmarshal(data, &file)
	if err != nil {
		return nil, fmt.Errorf("llm: parse models.json: %w", err)
	}
	for id, definition := range file.Models {
		if definition.ContextWindow <= 0 {
			return nil, fmt.Errorf("llm: model %q has invalid contextWindow", id)
		}
	}
	return file.Models, nil
}

func reasoningOptions(definition model, effort string) (map[string]any, error) {
	if effort == "" {
		return nil, nil
	}
	for _, level := range definition.Reasoning {
		if level.Effort == effort {
			return level.Options, nil
		}
	}
	return nil, fmt.Errorf("llm: model %q does not support reasoning effort %q", definition.ID, effort)
}

func newModel(definition model, config providerConfig) (provider.LanguageModel, error) {
	switch definition.Provider {
	case "deepseek":
		options := []deepseek.Option{deepseek.WithAPIKey(config.APIKey)}
		if config.BaseURL != "" {
			options = append(options, deepseek.WithBaseURL(config.BaseURL))
		}
		return deepseek.Chat(definition.ID, options...), nil
	case "google":
		options := []google.Option{google.WithAPIKey(config.APIKey)}
		if config.BaseURL != "" {
			options = append(options, google.WithBaseURL(config.BaseURL))
		}
		return google.Chat(definition.ID, options...), nil
	default:
		return nil, fmt.Errorf("llm: provider %q is not supported", definition.Provider)
	}
}
