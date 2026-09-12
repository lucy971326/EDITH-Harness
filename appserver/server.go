package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"harness/kernel/agents"
	"harness/kernel/commands"
	"harness/kernel/events"
	"harness/kernel/llm"
	"harness/kernel/skills"
	"harness/products/harness"
)

// 活对象。应用唯一的接入服务，拥有方法表、页面监听和当前连接。
type Server struct {
	// 产品入口与公共能力；业务状态由产品和内核管理。
	harnessProduct *harness.Product
	events         *events.Registry
	models         *llm.Client
	agents         *agents.Service
	skills         skills.Skills
	commands       commands.Commands

	// 对外方法。
	methods map[string]registeredMethod

	// 服务生命周期；关闭后拒绝新调用和新连接。
	mu          sync.Mutex
	closed      bool
	active      sync.WaitGroup
	connections map[*Connection]struct{}

	// React 页面与 WebSocket 监听。
	httpServer *http.Server
	serveDone  chan error
	web        http.Handler
}

// 契约。登记表中的处理函数，已绑定运行时校验与类型转换。
type registeredMethod func(context.Context, json.RawMessage) (json.RawMessage, error)

// New 创建空服务；不启动监听、模型或后台任务。
func New() *Server {
	return &Server{
		methods:     make(map[string]registeredMethod),
		connections: make(map[*Connection]struct{}),
	}
}

// Register 绑定方法名和类型化处理函数；错误契约在组装时失败。
func Register[Input, Output any](server *Server, name string, handler func(context.Context, Input) (Output, error)) error {
	if server == nil || handler == nil {
		return fmt.Errorf("appserver: nil server or handler")
	}
	inputSchema, outputSchema, err := compileMethod[Input, Output](name)
	if err != nil {
		return err
	}
	method := boundMethod[Input, Output]{
		handler:      handler,
		inputSchema:  inputSchema,
		outputSchema: outputSchema,
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if server.closed {
		return fmt.Errorf("appserver: registration is closed")
	}
	if _, exists := server.methods[name]; exists {
		return fmt.Errorf("appserver: duplicate method %q", name)
	}
	server.methods[name] = method.Call
	return nil
}

// Call 校验并分发一次进程内请求，不自动重试。
func (s *Server) Call(ctx context.Context, name string, params json.RawMessage) (json.RawMessage, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, &Error{CodeConflict, "server is not accepting calls", nil}
	}
	handler, exists := s.methods[name]
	if !exists {
		s.mu.Unlock()
		return nil, &Error{CodeUnknownMethod, "unknown method", nil}
	}
	s.active.Add(1)
	s.mu.Unlock()
	defer s.active.Done()

	err := ctx.Err()
	if err != nil {
		return nil, &Error{CodeInternal, "request cancelled", err}
	}
	return handler(ctx, params)
}
