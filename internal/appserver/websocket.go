package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"harness/internal/appserver/internal/clientconn"

	"github.com/coder/websocket"
)

const maxRPCMessageBytes = 16 << 20

// serveHTTP 只区分 RPC 与静态页面。
func (s *Server) serveHTTP(web http.Handler, w http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/rpc":
		s.serveRPC(w, request)
	default:
		if web == nil {
			http.NotFound(w, request)
			return
		}
		web.ServeHTTP(w, request)
	}
}

// serveRPC 校验本机入口，升级 WebSocket，并维持一个 Client 连接。
func (s *Server) serveRPC(w http.ResponseWriter, request *http.Request) {
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

	if !s.lifecycle.begin() {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	defer s.lifecycle.end()

	socket, err := websocket.Accept(w, request, nil)
	if err != nil {
		return
	}
	socket.SetReadLimit(maxRPCMessageBytes)
	stream := &websocketObjectStream{ctx: request.Context(), socket: socket}
	connection := clientconn.New(s.lifecycle.context(), stream, s.prepareCall)
	connection.Run()
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
