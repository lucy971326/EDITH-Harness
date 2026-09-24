package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// 活对象。Registry 保存公开方法；只有显式登记的方法可被调用。
type Registry struct {
	mu        sync.RWMutex
	methods   map[string]Method
	scheduler *Scheduler
}

func NewRegistry() *Registry {
	return &Registry{
		methods:   make(map[string]Method),
		scheduler: newScheduler(),
	}
}

// Register 登记可并发执行的普通方法。
func Register[Input, Output any](registry *Registry, name string, handler func(context.Context, Input) (Output, error)) error {
	method, err := bind(name, handler, nil, nil)
	if err != nil {
		return err
	}
	return registry.register(name, method)
}

// RegisterSession 登记必须按 Session 顺序执行的写方法。
func RegisterSession[Input, Output any](registry *Registry, name string, sessionKey func(Input) string, handler func(context.Context, Input) (Output, error)) error {
	if registry == nil {
		return fmt.Errorf("appserver: nil registry")
	}
	if sessionKey == nil {
		return fmt.Errorf("appserver: nil session key")
	}
	method, err := bind(name, handler, registry.scheduler, sessionKey)
	if err != nil {
		return err
	}
	return registry.register(name, method)
}

func (r *Registry) register(name string, method Method) error {
	if r == nil {
		return fmt.Errorf("appserver: nil registry")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.methods[name]; exists {
		return fmt.Errorf("appserver: duplicate method %q", name)
	}
	r.methods[name] = method
	return nil
}

// Prepare 查找并准备一个公开方法；Session 方法会在这里保留到达顺序。
func (r *Registry) Prepare(ctx context.Context, name string, params json.RawMessage) PreparedCall {
	r.mu.RLock()
	handler, exists := r.methods[name]
	r.mu.RUnlock()
	if !exists {
		return failedCall(&Error{Code: CodeUnknownMethod, Message: "unknown method"})
	}
	err := ctx.Err()
	if err != nil {
		return failedCall(&Error{Code: CodeInternal, Message: "request cancelled", Cause: err})
	}
	return handler(ctx, params)
}
