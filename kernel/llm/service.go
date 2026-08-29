package llm

import (
	"context"
	"fmt"
	"strings"

	"harness/kernel/kinds"
)

// Service 解析 Setup 指定的模型，并把统一请求交给对应协议 Adapter。
type Service struct {
	config   Config
	catalog  Catalog
	adapters map[string]Adapter
}

// New 构造一份独立的 LLM Service。配置与 Catalog 会被复制，之后调用者的修改无效。
func New(cfg Config, catalog Catalog, adapters map[string]Adapter) (*Service, error) {
	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("llm: no providers configured")
	}

	copiedAdapters := make(map[string]Adapter, len(adapters))
	for api, adapter := range adapters {
		if api == "" {
			return nil, fmt.Errorf("llm: empty adapter API")
		}
		if adapter == nil {
			return nil, fmt.Errorf("llm: adapter %q is nil", api)
		}
		copiedAdapters[api] = adapter
	}

	return &Service{
		config:   cloneConfig(cfg),
		catalog:  cloneCatalog(catalog),
		adapters: copiedAdapters,
	}, nil
}

// Stream 使用 Setup 快照中的 Model 和 ReasoningEffort。没有每轮临时覆盖。
func (s *Service) Stream(ctx context.Context, setup kinds.Setup, req Request) (<-chan Chunk, error) {
	target, reasoning, adapter, err := s.resolve(setup)
	if err != nil {
		return nil, err
	}

	stream, err := adapter.Stream(ctx, Call{
		Target:    target,
		System:    req.System,
		Messages:  projectMessages(req.Messages, target.Vision),
		Reasoning: clonePatch(reasoning),
	})
	if err != nil {
		return nil, fmt.Errorf("llm: %s: %w", target.Key, err)
	}
	if stream == nil {
		return nil, fmt.Errorf("llm: %s: adapter returned nil stream", target.Key)
	}

	out := make(chan Chunk)
	go forward(stream, out)
	return out, nil
}

func (s *Service) resolve(setup kinds.Setup) (Target, Patch, Adapter, error) {
	providerName, modelName, err := splitModel(setup.Model)
	if err != nil {
		return Target{}, nil, nil, err
	}

	provider, ok := s.config.Providers[providerName]
	if !ok {
		return Target{}, nil, nil, fmt.Errorf("llm: provider %q is not configured", providerName)
	}
	model, ok := provider.Models[modelName]
	if !ok {
		return Target{}, nil, nil, fmt.Errorf("llm: model %q is not enabled for provider %q", modelName, providerName)
	}
	if provider.Catalog == "" {
		return Target{}, nil, nil, fmt.Errorf("llm: provider %q has no catalog", providerName)
	}
	catalogProvider, ok := s.catalog.Providers[provider.Catalog]
	if !ok {
		return Target{}, nil, nil, fmt.Errorf("llm: catalog provider %q not found", provider.Catalog)
	}
	facts, ok := catalogProvider.Models[modelName]
	if !ok {
		return Target{}, nil, nil, fmt.Errorf("llm: catalog model %q/%q not found", provider.Catalog, modelName)
	}

	api := model.API
	if api == "" {
		api = provider.API
	}
	if api == "" {
		return Target{}, nil, nil, fmt.Errorf("llm: model %q has no API", setup.Model)
	}
	adapter, ok := s.adapters[api]
	if !ok {
		return Target{}, nil, nil, fmt.Errorf("llm: API %q for model %q is not compiled", api, setup.Model)
	}

	var reasoning Patch
	if setup.ReasoningEffort != "" {
		var supported bool
		reasoning, supported = facts.Reasoning[setup.ReasoningEffort]
		if !supported {
			return Target{}, nil, nil, fmt.Errorf("llm: model %q does not support reasoning effort %q", setup.Model, setup.ReasoningEffort)
		}
	}

	remoteID := model.ID
	if remoteID == "" {
		remoteID = modelName
	}
	return Target{
		Key:       setup.Model,
		Provider:  providerName,
		Model:     modelName,
		RemoteID:  remoteID,
		API:       api,
		BaseURL:   provider.BaseURL,
		APIKeyEnv: provider.APIKeyEnv,
		Vision:    facts.Vision,
	}, reasoning, adapter, nil
}

func splitModel(value string) (string, string, error) {
	provider, model, ok := strings.Cut(value, "/")
	if !ok || provider == "" || model == "" || strings.Contains(model, "/") {
		return "", "", fmt.Errorf("llm: model must be provider/model, got %q", value)
	}
	return provider, model, nil
}

func forward(in <-chan Chunk, out chan<- Chunk) {
	defer close(out)

	terminal := false
	for chunk := range in {
		if terminal {
			continue
		}
		if chunk.Done && chunk.Err != nil {
			out <- Chunk{Err: fmt.Errorf("llm: adapter emitted Done and Err together")}
			terminal = true
			continue
		}
		out <- chunk
		if chunk.Done || chunk.Err != nil {
			terminal = true
		}
	}
	if !terminal {
		out <- Chunk{Err: fmt.Errorf("llm: adapter stream ended without terminal chunk")}
	}
}
