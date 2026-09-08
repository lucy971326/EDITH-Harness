package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// 活对象。入口直接拥有的 RPC 登记处与分发器，不负责 HTTP 或 WebSocket 收发。
type RPCServer struct {
	mu      sync.RWMutex
	methods map[string]registeredMethod
	frozen  bool
	closed  bool
	active  sync.WaitGroup
}

// 数据。一条完成校验编译并绑定处理函数的接口。
type registeredMethod struct {
	definition Definition
	call       func(context.Context, json.RawMessage) (json.RawMessage, error)
}

// New 创建空登记处；不启动连接、模型或后台任务。
func New() *RPCServer { return &RPCServer{methods: make(map[string]registeredMethod)} }

// Register 绑定类型化声明和处理函数；错误契约在组装时失败。
func Register[Input, Output any](s *RPCServer, method Method[Input, Output], handler Handler[Input, Output]) error {
	if s == nil || handler == nil {
		return fmt.Errorf("appserver: nil server or handler")
	}
	definition, inputSchema, outputSchema, err := describe(method)
	if err != nil {
		return err
	}
	bound := boundMethod[Input, Output]{
		handler:      handler,
		inputSchema:  inputSchema,
		outputSchema: outputSchema,
	}
	entry := registeredMethod{definition: definition, call: bound.Call}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.frozen {
		return fmt.Errorf("appserver: registration is closed")
	}
	if _, exists := s.methods[method.Name]; exists {
		return fmt.Errorf("appserver: duplicate method %q", method.Name)
	}
	s.methods[method.Name] = entry
	return nil
}

// Freeze 结束组装；此后目录只读，才允许业务调用。
func (s *RPCServer) Freeze() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("appserver: closed")
	}
	s.frozen = true
	return nil
}

// Catalog 返回独立副本；调用方不能修改运行时契约。
func (s *RPCServer) Catalog() []Definition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 按方法名排序，保证目录顺序稳定。
	names := make([]string, 0, len(s.methods))
	for name := range s.methods {
		names = append(names, name)
	}
	sort.Strings(names)
	catalog := make([]Definition, 0, len(names))
	for _, name := range names {
		entry := s.methods[name]
		definition := entry.definition
		definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
		definition.OutputSchema = append(json.RawMessage(nil), definition.OutputSchema...)
		catalog = append(catalog, definition)
	}
	return catalog
}

// Call 校验并分发一次进程内请求，不自动重试。
func (s *RPCServer) Call(ctx context.Context, name string, params json.RawMessage) (json.RawMessage, error) {
	s.mu.Lock()
	ready := s.frozen && !s.closed
	entry, exists := s.methods[name]
	if ready && exists {
		s.active.Add(1)
	}
	s.mu.Unlock()
	if !ready {
		return nil, &Error{CodeConflict, "server is not accepting calls", nil}
	}
	if !exists {
		return nil, &Error{CodeUnknownMethod, "unknown method", nil}
	}
	defer s.active.Done()
	err := ctx.Err()
	if err != nil {
		return nil, &Error{CodeInternal, "request cancelled", err}
	}
	return entry.call(ctx, params)
}

// Close 幂等关闭准入；本批没有监听器或连接资源。入口在关闭 Host 前调用。
func (s *RPCServer) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.active.Wait()
	return nil
}
