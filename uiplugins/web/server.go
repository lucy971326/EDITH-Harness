package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"harness/agents"
	"harness/commands"
	"harness/llm"
	"harness/presets"
	"harness/projects"
	"harness/session"
	"harness/tools"
)

// service 把领域能力组合成一个只监听本机的网页入口。
type service struct {
	projects projects.Service
	presets  presets.Service
	books    *session.Store
	agents   agents.Service
	llm      *llm.Service
	tools    *tools.Registry
	commands commands.Service
	updates  *updateHub
	picker   directoryPicker

	mu     sync.Mutex
	server *http.Server
}

func newService(projectService projects.Service, presetService presets.Service, books *session.Store, agentService agents.Service, llmService *llm.Service, toolRegistry *tools.Registry, commandService commands.Service, updates *updateHub, picker directoryPicker) *service {
	return &service{
		projects: projectService,
		presets:  presetService,
		books:    books,
		agents:   agentService,
		llm:      llmService,
		tools:    toolRegistry,
		commands: commandService,
		updates:  updates,
		picker:   picker,
	}
}

// Run 启动本机网页服务，直到上下文取消或 App 收摊。
func (s *service) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		return fmt.Errorf("Web UI 无法监听 127.0.0.1:8080：%w", err)
	}
	server := &http.Server{Handler: s.routes()}
	s.mu.Lock()
	if s.server != nil {
		s.mu.Unlock()
		_ = listener.Close()
		return fmt.Errorf("Web UI 已经启动")
	}
	s.server = server
	s.mu.Unlock()

	fmt.Println("Web UI 已启动：http://127.0.0.1:8080")
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("Web UI 已停止：%w", err)
}

// Close 关闭正在监听的 HTTP 服务；尚未启动时是空操作。
func (s *service) Close() {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.mu.Unlock()
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
