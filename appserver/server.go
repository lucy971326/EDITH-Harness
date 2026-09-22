package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	internalrpc "harness/appserver/internal/rpc"
	"harness/appserver/internal/workspacepicker"
	"harness/kernel/agents"
	"harness/kernel/approvals"
	"harness/kernel/commands"
	"harness/kernel/events"
	"harness/kernel/llm"
	"harness/kernel/machine"
	"harness/kernel/runner"
	"harness/kernel/skills"
	"harness/products/harness"
)

// 活对象。应用唯一的接入服务，拥有方法表、页面监听和当前连接。
type Server struct {
	// 产品入口与公共能力；业务状态由产品和内核管理。
	harnessProduct  *harness.Product
	approvals       *approvals.Service
	runner          *runner.Runner
	events          *events.Registry
	models          *llm.Client
	agents          *agents.Service
	skills          skills.Skills
	commands        commands.Commands
	filesystem      machine.FileSystem
	pathSearcher    machine.PathSearcher
	commandExec     *commandExecManager
	workspacePicker workspacepicker.Picker

	// 对外方法及同一 Session 的写请求顺序。
	methods *internalrpc.Registry

	// 服务生命周期。
	lifecycle serverLifecycle
	closeOnce sync.Once
	closeErr  error

	// React 页面与 WebSocket 监听。
	listener serverListener
}

// New 创建空服务；不启动监听、模型或后台任务。
func New() *Server {
	return &Server{
		methods:         internalrpc.NewRegistry(),
		workspacePicker: workspacepicker.Pick,
		lifecycle:       newServerLifecycle(),
	}
}

// Register 绑定方法名和类型化处理函数；错误契约在组装时失败。
func Register[Input, Output any](server *Server, name string, handler func(context.Context, Input) (Output, error)) error {
	if server == nil || handler == nil {
		return fmt.Errorf("appserver: nil server or handler")
	}
	if !server.lifecycle.begin() {
		return fmt.Errorf("appserver: registration is closed")
	}
	defer server.lifecycle.end()
	return internalrpc.Register(server.methods, name, handler)
}

// registerSession 只用于确实需要按 Session 排序的写方法。
func registerSession[Input, Output any](server *Server, name string, sessionKey func(Input) string, handler func(context.Context, Input) (Output, error)) error {
	if server == nil || handler == nil || sessionKey == nil {
		return fmt.Errorf("appserver: nil server, handler or session key")
	}
	if !server.lifecycle.begin() {
		return fmt.Errorf("appserver: registration is closed")
	}
	defer server.lifecycle.end()
	return internalrpc.RegisterSession(server.methods, name, sessionKey, handler)
}

// Call 校验并分发一次进程内请求，不自动重试。
func (s *Server) Call(ctx context.Context, name string, params json.RawMessage) (json.RawMessage, error) {
	return s.prepareCall(ctx, name, params)()
}

// prepareCall 在网络接收协程中完成校验与排队；返回的函数等待业务结果。
func (s *Server) prepareCall(ctx context.Context, name string, params json.RawMessage) internalrpc.PreparedCall {
	if !s.lifecycle.begin() {
		return func() (json.RawMessage, error) {
			return nil, &Error{Code: CodeConflict, Message: "server is not accepting calls"}
		}
	}

	call := s.methods.Prepare(ctx, name, params)
	return func() (json.RawMessage, error) {
		defer s.lifecycle.end()
		return call()
	}
}
