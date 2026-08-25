package llm

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"harness/chat"
)

// Service 是插座排：登记了一排适配器，按名字路由。
// 一家的适配器坏了换另一家，消费方一个字都不用改。
type Service struct {
	mu       sync.Mutex
	adapters map[string]Adapter
}

// NewService 建一个空的插座排。
func NewService() *Service {
	return &Service{adapters: make(map[string]Adapter)}
}

// Register 插上一个适配器；同名重复插是组装错误。
func (s *Service) Register(adapter Adapter) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := adapter.Name()
	_, taken := s.adapters[name]
	if taken {
		return fmt.Errorf("插座 %s 已被占用", name)
	}
	s.adapters[name] = adapter
	return nil
}

// Providers 按名字列出已安装适配器及其会话模型选择目录。
func (s *Service) Providers() []ProviderInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	providers := make([]ProviderInfo, 0, len(s.adapters))
	for name, adapter := range s.adapters {
		info := ProviderInfo{Name: name}
		catalog, ok := adapter.(ProviderCatalog)
		if ok {
			info = catalog.ProviderInfo()
		}
		if info.Name == "" {
			info.Name = name
		}
		info.Models = cloneModels(info.Models)
		providers = append(providers, info)
	}
	sort.Slice(providers, func(i int, j int) bool {
		return providers[i].Name < providers[j].Name
	})
	return providers
}

// ValidateSelection 检查服务商、模型和思考档位是否是适配器声明的有效组合。
func (s *Service) ValidateSelection(provider string, model string, thinking string) error {
	s.mu.Lock()
	adapter, exists := s.adapters[provider]
	s.mu.Unlock()
	if !exists {
		return fmt.Errorf("模型服务商 %s 没有安装", provider)
	}
	catalog, hasCatalog := adapter.(ProviderCatalog)
	if !hasCatalog {
		return nil
	}
	info := catalog.ProviderInfo()
	for _, candidate := range info.Models {
		if candidate.ID != model {
			continue
		}
		for _, level := range candidate.ThinkingLevels {
			if level == thinking {
				return nil
			}
		}
		return fmt.Errorf("模型 %s 不支持思考档位 %s", model, thinking)
	}
	return fmt.Errorf("服务商 %s 没有模型 %s", provider, model)
}

// Validate 检查一段会话选中的模型组合是否可用。
func (s *Service) Validate(selection Selection) error {
	return s.ValidateSelection(selection.Provider, selection.Model, selection.Thinking)
}

// DefaultSelection 返回已安装目录中的第一个可用模型组合；没有目录时返回错误。
func (s *Service) DefaultSelection() (Selection, error) {
	for _, provider := range s.Providers() {
		for _, model := range provider.Models {
			if len(model.ThinkingLevels) == 0 {
				continue
			}
			return Selection{Provider: provider.Name, Model: model.ID, Thinking: model.ThinkingLevels[0]}, nil
		}
	}
	return Selection{}, fmt.Errorf("没有已安装且声明模型目录的服务商")
}

// Complete 按请求指定的服务商发一次请求，不关心逐字（要逐字用 Stream）。
func (s *Service) Complete(ctx context.Context, req Request) (Reply, error) {
	return s.Stream(ctx, req, nil)
}

// Stream 按请求指定的服务商发一次请求，模型吐的每小截字实时递给 onDelta。
func (s *Service) Stream(ctx context.Context, req Request, onDelta func(chat.Delta)) (Reply, error) {
	if req.Provider == "" {
		return Reply{}, NewError("", ErrBadRequest, "模型请求缺少 provider")
	}
	if req.Model == "" {
		return Reply{}, NewError(req.Provider, ErrBadRequest, "模型请求缺少 model")
	}
	if req.Thinking == "" {
		return Reply{}, NewError(req.Provider, ErrBadRequest, "模型请求缺少 thinking")
	}
	err := s.ValidateSelection(req.Provider, req.Model, req.Thinking)
	if err != nil {
		return Reply{}, NewError(req.Provider, ErrBadRequest, err.Error())
	}
	return s.stream(ctx, req, onDelta)
}

func cloneModels(models []ModelInfo) []ModelInfo {
	copied := make([]ModelInfo, len(models))
	for index, model := range models {
		copied[index] = model
		copied[index].ThinkingLevels = append([]string(nil), model.ThinkingLevels...)
	}
	return copied
}

// stream 路由到插座并包错误——出口只有这一个，统一长相在这保证。
func (s *Service) stream(ctx context.Context, req Request, onDelta func(chat.Delta)) (Reply, error) {
	s.mu.Lock()
	adapter, exists := s.adapters[req.Provider]
	s.mu.Unlock()

	if !exists {
		return Reply{}, NewError(req.Provider, ErrBadRequest, fmt.Sprintf("插座 %s 没登记过", req.Provider))
	}

	reply, err := adapter.Stream(ctx, req, onDelta)
	if err != nil {
		return Reply{}, wrapError(req.Provider, err)
	}
	return reply, nil
}
