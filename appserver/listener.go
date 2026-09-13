package appserver

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

// serverListener 拥有 React 页面与 WebSocket 共用的 HTTP 监听器。
type serverListener struct {
	mu     sync.Mutex
	server *http.Server
	done   chan error
}

// Listen 在数字回环地址启动 React 页面与 WebSocket RPC 入口。
func (s *Server) Listen(address string, web http.Handler) (string, error) {
	addr, err := netip.ParseAddrPort(address)
	if err != nil || !addr.Addr().IsLoopback() {
		return "", fmt.Errorf("appserver: loopback address required")
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		s.serveHTTP(web, w, request)
	})
	return s.listener.start(&s.lifecycle, address, handler)
}

func (l *serverListener) start(lifecycle *serverLifecycle, address string, handler http.Handler) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !lifecycle.begin() {
		return "", fmt.Errorf("appserver: invalid listener state")
	}
	defer lifecycle.end()
	if l.server != nil {
		return "", fmt.Errorf("appserver: invalid listener state")
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return "", err
	}
	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	done := make(chan error, 1)
	l.server = httpServer
	l.done = done
	go func() { done <- httpServer.Serve(listener) }()
	return "http://" + listener.Addr().String(), nil
}

// Close 拒绝新调用，断开 Client 并等待请求结束；不停止已接受的 Run。
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.lifecycle.close()
		s.closeErr = s.listener.close()
		s.lifecycle.wait()
	})
	return s.closeErr
}

func (l *serverListener) close() error {
	l.mu.Lock()
	httpServer, done := l.server, l.done
	l.mu.Unlock()
	if httpServer == nil {
		return nil
	}

	err := httpServer.Close()
	serveErr := <-done
	if errors.Is(serveErr, http.ErrServerClosed) {
		return err
	}
	return errors.Join(err, serveErr)
}
