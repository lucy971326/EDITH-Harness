// Package webclient 提供 Web 的 HTTP 与 WebSocket 接入。
package webclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"harness/internal/appserver"
)

// 活对象。Web 入口拥有 HTTP 监听与升级后的传输；RPC 状态归 appserver。
type Server struct {
	rpc    *appserver.Server
	assets http.Handler

	// mu 串行化启动、关闭和 HTTP 准入；关闭后不再增加 active。
	mu     sync.Mutex
	closed bool
	active sync.WaitGroup
	server *http.Server
	cancel context.CancelFunc
	done   chan error
}

func New(rpc *appserver.Server, assets http.Handler) (*Server, error) {
	if rpc == nil {
		return nil, fmt.Errorf("web: RPC server required")
	}
	return &Server{rpc: rpc, assets: assets}, nil
}

// Listen 只允许数字回环地址，静态页面与 RPC 共用一个监听器。
func (s *Server) Listen(address string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.server != nil {
		return "", fmt.Errorf("web: invalid listener state")
	}
	addr, err := netip.ParseAddrPort(address)
	if err != nil || !addr.Addr().IsLoopback() {
		return "", fmt.Errorf("web: loopback address required")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.server = &http.Server{
		Handler:           s,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.done = make(chan error, 1)
	go func() { s.done <- s.server.Serve(listener) }()
	return "http://" + listener.Addr().String(), nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	s.active.Add(1)
	s.mu.Unlock()
	defer s.active.Done()
	if request.URL.Path == "/rpc" {
		s.serveRPC(w, request)
		return
	}
	if s.assets == nil {
		http.NotFound(w, request)
		return
	}
	s.assets.ServeHTTP(w, request)
}

// Close 先停止准入，再断开传输并等待 RPC 清理；后台服务由入口随后关闭。
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.server == nil {
		return nil
	}
	// http.Server.Close 不处理已升级的连接，须一并取消其读写上下文。
	s.cancel()
	err := s.server.Close()
	serveErr := <-s.done
	s.active.Wait()
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(err, serveErr)
}
