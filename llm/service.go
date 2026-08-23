package llm

import (
	"context"
	"fmt"
	"sync"

	"harness/chat"
)

// Service 是插座排：登记了一排适配器，按名字路由。
// 一家的适配器坏了换另一家，消费方一个字都不用改。
type Service struct {
	mu          sync.Mutex
	adapters    map[string]Adapter
	defaultName string // 默认插座名，Complete 用它
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

// SetDefault 设默认插座；不设就调 Complete 是组装不完整。
func (s *Service) SetDefault(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.adapters[name]
	if !exists {
		return fmt.Errorf("插座 %s 没登记过，设不了默认", name)
	}
	s.defaultName = name
	return nil
}

// Complete 用默认插座发一次请求，不关心逐字（要逐字用 Stream）。
func (s *Service) Complete(ctx context.Context, req Request) (Reply, error) {
	s.mu.Lock()
	name := s.defaultName
	s.mu.Unlock()

	if name == "" {
		return Reply{}, fmt.Errorf("没设默认插座，先 SetDefault 或用 CompleteWith 指明哪家")
	}
	return s.CompleteWith(ctx, name, req)
}

// CompleteWith 指明哪家插座发一次请求。
func (s *Service) CompleteWith(ctx context.Context, provider string, req Request) (Reply, error) {
	return s.streamWith(ctx, provider, req, nil)
}

// Stream 用默认插座发一次请求，模型吐的每小截字实时递给 onDelta。
func (s *Service) Stream(ctx context.Context, req Request, onDelta func(chat.Delta)) (Reply, error) {
	s.mu.Lock()
	name := s.defaultName
	s.mu.Unlock()

	if name == "" {
		return Reply{}, fmt.Errorf("没设默认插座，先 SetDefault 或用 StreamWith 指明哪家")
	}
	return s.StreamWith(ctx, name, req, onDelta)
}

// StreamWith 指明哪家插座，边收边递字。
func (s *Service) StreamWith(ctx context.Context, provider string, req Request, onDelta func(chat.Delta)) (Reply, error) {
	return s.streamWith(ctx, provider, req, onDelta)
}

// streamWith 路由到插座并包错误——出口只有这一个，统一长相在这保证。
func (s *Service) streamWith(ctx context.Context, provider string, req Request, onDelta func(chat.Delta)) (Reply, error) {
	s.mu.Lock()
	adapter, exists := s.adapters[provider]
	s.mu.Unlock()

	if !exists {
		return Reply{}, fmt.Errorf("插座 %s 没登记过", provider)
	}

	reply, err := adapter.Stream(ctx, req, onDelta)
	if err != nil {
		return Reply{}, wrapError(provider, err)
	}
	return reply, nil
}
