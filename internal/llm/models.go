package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"harness/internal/persist"
)

// 数据。models.json 的根对象。
type modelFile struct {
	Version int              `json:"version,omitempty"`
	Models  map[string]model `json:"models"`
	Hidden  []string         `json:"hidden,omitempty"`
}

// 数据。一条模型定义。
type model struct {
	Provider      string          `json:"provider"`
	Protocol      string          `json:"protocol,omitempty"`
	ID            string          `json:"id"`
	ContextWindow int             `json:"contextWindow"`
	MaxOutput     int             `json:"maxOutput,omitempty"`
	Manual        bool            `json:"-"`
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

func (levels reasoningLevels) MarshalJSON() ([]byte, error) {
	var body bytes.Buffer
	body.WriteByte('{')
	for index, level := range levels {
		if index > 0 {
			body.WriteByte(',')
		}
		name, err := json.Marshal(level.Effort)
		if err != nil {
			return nil, err
		}
		options, err := json.Marshal(level.Options)
		if err != nil {
			return nil, err
		}
		body.Write(name)
		body.WriteByte(':')
		body.Write(options)
	}
	body.WriteByte('}')
	return body.Bytes(), nil
}

func loadModels() (map[string]model, error) {
	if catalogError != nil {
		return nil, catalogError
	}
	out := make(map[string]model, len(catalog.Models))
	for key, value := range catalog.Models {
		out[key] = value
	}
	return out, nil
}

func loadModelsFile(files *persist.Files) (map[string]model, []byte, error) {
	body, err := files.Read("models.json")
	if errors.Is(err, os.ErrNotExist) {
		body = []byte(`{"version":2,"models":{}}`)
		err = files.Write("models.json", body)
	}
	if err != nil {
		return nil, nil, err
	}
	models, err := parseModels(body)
	return models, body, err
}

func parseModels(data []byte) (map[string]model, error) {
	var file modelFile
	err := json.Unmarshal(data, &file)
	if err != nil {
		return nil, fmt.Errorf("llm: parse models.json: %w", err)
	}
	if file.Version != 2 {
		return nil, fmt.Errorf("llm: unsupported models file version %d", file.Version)
	}
	out, err := loadModels()
	if err != nil {
		return nil, err
	}
	for key, value := range file.Models {
		if value.Protocol != "" && !validProtocol(value.Protocol) {
			return nil, fmt.Errorf("llm: unsupported model protocol %q", value.Protocol)
		}
		if value.ContextWindow <= 0 || value.MaxOutput < 0 || value.MaxOutput > value.ContextWindow {
			return nil, fmt.Errorf("llm: model %q has invalid token limits", key)
		}
		value.Manual = true
		out[key] = value
	}
	for _, key := range file.Hidden {
		delete(out, key)
	}
	return out, nil
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

func protocolFor(id string, config providerConfig) string {
	if builtInProvider(id) {
		return catalog.Providers[id].Protocol
	}
	if config.Protocol != "" {
		return config.Protocol
	}
	return catalog.Providers[id].Protocol
}

// 混合网关按模型选择协议，其余沿用供应商协议。
func modelProtocol(definition model, config providerConfig) string {
	if definition.Protocol != "" {
		return definition.Protocol
	}
	return protocolFor(definition.Provider, config)
}

func validProtocol(value string) bool {
	switch value {
	case "openai-chat", "openai-responses", "anthropic", "openai-codex":
		return true
	}
	return false
}

func settingsProtocol(id string, config providerConfig) string {
	if format := catalog.Providers[id].ThinkingFormat; format != "" {
		return format
	}
	return protocolFor(id, config)
}
