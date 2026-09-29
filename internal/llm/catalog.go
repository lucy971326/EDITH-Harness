package llm

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed catalog.json
var catalogJSON []byte

type modelCatalog struct {
	Providers map[string]ProviderPreset `json:"providers"`
	Models    map[string]model          `json:"models"`
}

var catalog, catalogError = readCatalog()

func readCatalog() (modelCatalog, error) {
	var result modelCatalog
	err := json.Unmarshal(catalogJSON, &result)
	if err != nil {
		return result, err
	}
	for key, model := range result.Models {
		preset, ok := result.Providers[model.Provider]
		if !ok || preset.BaseURL == "" || model.ContextWindow <= 0 || model.MaxOutput <= 0 || len(model.Reasoning) == 0 {
			return result, fmt.Errorf("llm: invalid generated catalog entry %q", key)
		}
	}
	return result, nil
}
