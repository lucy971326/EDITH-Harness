package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// 活对象。入口直接拥有的 RPC 登记处与分发器，不负责 HTTP 或 WebSocket 收发。
type RPCServer struct {
	// 方法登记：按名称查找处理函数。
	methods map[string]registeredMethod

	// 关闭收尾：拒绝新调用，等待已接收的调用结束。
	closed bool
	active sync.WaitGroup

	// 并发协调：保护登记表和状态，让调用准入与关闭互斥。
	mu sync.RWMutex
}

// 契约。登记表中的处理函数，已绑定运行时校验与类型转换。
type registeredMethod func(context.Context, json.RawMessage) (json.RawMessage, error)

// New 创建空登记处；不启动连接、模型或后台任务。
func New() *RPCServer { return &RPCServer{methods: make(map[string]registeredMethod)} }

// Register 绑定类型化声明和处理函数；错误契约在组装时失败。
func Register[Input, Output any](s *RPCServer, method Method[Input, Output], handler Handler[Input, Output]) error {
	if s == nil || handler == nil {
		return fmt.Errorf("appserver: nil server or handler")
	}
	inputSchema, outputSchema, err := compileMethod(method)
	if err != nil {
		return err
	}
	bound := boundMethod[Input, Output]{
		handler:      handler,
		inputSchema:  inputSchema,
		outputSchema: outputSchema,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("appserver: registration is closed")
	}
	if _, exists := s.methods[method.Name]; exists {
		return fmt.Errorf("appserver: duplicate method %q", method.Name)
	}
	s.methods[method.Name] = bound.Call
	return nil
}

// Call 校验并分发一次进程内请求，不自动重试。
func (s *RPCServer) Call(ctx context.Context, name string, params json.RawMessage) (json.RawMessage, error) {
	handler, err := s.beginCall(name)
	if err != nil {
		return nil, err
	}
	defer s.active.Done()

	err = ctx.Err()
	if err != nil {
		return nil, &Error{CodeInternal, "request cancelled", err}
	}
	return handler(ctx, params)
}

// beginCall 在同一把锁内完成准入、查方法与计数；成功后由 Call 负责 Done。
func (s *RPCServer) beginCall(name string) (registeredMethod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, &Error{CodeConflict, "server is not accepting calls", nil}
	}
	handler, exists := s.methods[name]
	if !exists {
		return nil, &Error{CodeUnknownMethod, "unknown method", nil}
	}
	s.active.Add(1)
	return handler, nil
}

// Close 幂等关闭方法准入并等待调用；入口先关闭 WebSocketServer，最后关闭 Host。
func (s *RPCServer) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.active.Wait()
	return nil
}
