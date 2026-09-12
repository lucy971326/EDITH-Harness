package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/coder/websocket"
)

const maxRPCMessageBytes = 16 << 20

// Listen 在数字回环地址启动唯一的 WebSocket RPC 入口。
func (s *Server) Listen(address string) (string, error) {
	addr, err := netip.ParseAddrPort(address)
	if err != nil || !addr.Addr().IsLoopback() {
		return "", fmt.Errorf("appserver: loopback address required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.httpServer != nil {
		return "", fmt.Errorf("appserver: invalid listener state")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return "", err
	}
	s.httpServer = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	s.serveDone = make(chan error, 1)
	go func() { s.serveDone <- s.httpServer.Serve(listener) }()
	return "ws://" + listener.Addr().String() + "/rpc", nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/rpc" {
		http.NotFound(w, request)
		return
	}
	host, _, err := net.SplitHostPort(request.Host)
	if err != nil {
		http.Error(w, "invalid host", http.StatusForbidden)
		return
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !addr.IsLoopback() {
		http.Error(w, "loopback host required", http.StatusForbidden)
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	s.active.Add(1)
	s.mu.Unlock()
	defer s.active.Done()

	socket, err := websocket.Accept(w, request, nil)
	if err != nil {
		return
	}
	socket.SetReadLimit(maxRPCMessageBytes)
	stream := &websocketObjectStream{ctx: request.Context(), socket: socket}
	connection := newConnection(request.Context(), stream, s)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		connection.disconnect()
		connection.run()
		return
	}
	s.connections[connection] = struct{}{}
	s.mu.Unlock()

	connection.run()
	s.mu.Lock()
	delete(s.connections, connection)
	s.mu.Unlock()
}

// Close 拒绝新调用，断开 Client 并等待请求结束；不停止已接受的 Run。
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	for connection := range s.connections {
		connection.disconnect()
	}
	httpServer, serveDone := s.httpServer, s.serveDone
	s.mu.Unlock()

	var err error
	if httpServer != nil {
		err = httpServer.Close()
	}
	s.active.Wait()
	if serveDone == nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.serveDone == nil {
		return err
	}
	serveErr := <-serveDone
	s.serveDone = nil
	if errors.Is(serveErr, http.ErrServerClosed) {
		return err
	}
	return errors.Join(err, serveErr)
}

// websocketObjectStream 只把 coder/websocket 消息交给 JSON-RPC 库。
type websocketObjectStream struct {
	ctx    context.Context
	socket *websocket.Conn
}

func (s *websocketObjectStream) ReadObject(value any) error {
	kind, raw, err := s.socket.Read(s.ctx)
	if err != nil {
		return err
	}
	if kind != websocket.MessageText {
		return fmt.Errorf("appserver: text message required")
	}
	return json.Unmarshal(raw, value)
}

func (s *websocketObjectStream) WriteObject(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	return s.socket.Write(ctx, websocket.MessageText, raw)
}

func (s *websocketObjectStream) Close() error {
	return s.socket.CloseNow()
}
