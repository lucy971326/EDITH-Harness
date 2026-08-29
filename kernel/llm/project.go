package llm

import "harness/kernel/session"

const imageUnsupportedText = "当前模型不支持图片查看"

func projectMessages(messages []session.Message, vision bool) []session.Message {
	out := make([]session.Message, len(messages))
	for i, message := range messages {
		out[i].Role = message.Role
		out[i].Blocks = make([]session.Block, len(message.Blocks))
		for j, block := range message.Blocks {
			if block.Kind == "image" && !vision {
				out[i].Blocks[j] = session.Block{Kind: "text", Text: imageUnsupportedText}
				continue
			}

			copied := block
			if block.Tool != nil {
				tool := *block.Tool
				copied.Tool = &tool
			}
			if block.Media != nil {
				media := *block.Media
				copied.Media = &media
			}
			out[i].Blocks[j] = copied
		}
	}
	return out
}

func clonePatch(in Patch) Patch {
	if in == nil {
		return nil
	}
	out := make(Patch, len(in))
	for key, value := range in {
		out[key] = cloneJSON(value)
	}
	return out
}

func cloneJSON(in any) any {
	switch value := in.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = cloneJSON(item)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = cloneJSON(item)
		}
		return out
	default:
		return value
	}
}

func cloneConfig(in Config) Config {
	out := Config{Providers: make(map[string]ProviderConfig, len(in.Providers))}
	for name, provider := range in.Providers {
		copied := provider
		copied.Models = make(map[string]ModelConfig, len(provider.Models))
		for model, config := range provider.Models {
			copied.Models[model] = config
		}
		out.Providers[name] = copied
	}
	return out
}

func cloneCatalog(in Catalog) Catalog {
	out := Catalog{Providers: make(map[string]CatalogProvider, len(in.Providers))}
	for name, provider := range in.Providers {
		copiedProvider := CatalogProvider{Models: make(map[string]CatalogModel, len(provider.Models))}
		for model, facts := range provider.Models {
			copiedFacts := facts
			copiedFacts.Reasoning = make(map[string]Patch, len(facts.Reasoning))
			for effort, patch := range facts.Reasoning {
				copiedFacts.Reasoning[effort] = clonePatch(patch)
			}
			copiedProvider.Models[model] = copiedFacts
		}
		out.Providers[name] = copiedProvider
	}
	return out
}
