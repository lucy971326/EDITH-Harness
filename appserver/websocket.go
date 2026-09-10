package appserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// 活对象。本机 WebSocket 接入服务；入口创建并在 RPCServer、Host 之前关闭。
type WebSocketServer struct {
	// 固定依赖。
	RPC *RPCServer

	// 监听生命周期；mu 保护以下运行状态。
	mu         sync.Mutex
	closed     bool
	httpServer *http.Server
	serveDone  chan error

	// 连接生命周期；关闭时先取消，再等待全部退出。
	connections map[*Connection]struct{}
	active      sync.WaitGroup
}

// Listen 只允许数字回环地址；不创建第二个 Host 或业务登记处。
func (s *WebSocketServer) Listen(address string) (string, error) {
	addr, err := netip.ParseAddrPort(address)
	if err != nil || !addr.Addr().IsLoopback() {
		return "", fmt.Errorf("appserver: loopback address required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.httpServer != nil || s.RPC == nil {
		return "", fmt.Errorf("appserver: invalid listener state")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return "", err
	}
	s.httpServer = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	s.serveDone = make(chan error, 1)
	go s.serve(listener)
	return "ws://" + listener.Addr().String() + "/rpc", nil
}

func (s *WebSocketServer) serve(listener net.Listener) { s.serveDone <- s.httpServer.Serve(listener) }

func (s *WebSocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rpc" {
		http.NotFound(w, r)
		return
	}
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		http.Error(w, "invalid host", http.StatusForbidden)
		return
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !addr.IsLoopback() {
		http.Error(w, "loopback host required", http.StatusForbidden)
		return
	}
	if !s.beginConnection() {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	defer s.active.Done()
	// 默认拒绝跨 Origin 浏览器请求；无 Origin 的本机 Client 仍必须完成协议初始化。
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	c := &Connection{
		ctx:           ctx,
		cancel:        cancel,
		outgoing:      make(chan []byte, queueLimit),
		subscriptions: make(map[string]*Subscription),
	}
	c.initializeTimer = time.AfterFunc(10*time.Second, cancel)
	if !s.trackConnection(c) {
		c.close()
		return
	}
	defer s.remove(c)
	peer := websocketClientConnection{
		connection: c,
		socket:     ws,
		rpc:        s.RPC,
		incoming:   make(chan []byte, 32),
	}
	peer.run()
}

// beginConnection 在升级前计入在途连接，关闭时也要等待握手结束。
func (s *WebSocketServer) beginConnection() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.RPC == nil {
		return false
	}
	s.active.Add(1)
	return true
}

// trackConnection 握手期间可能已关闭；只有仍在接入时才登记连接。
func (s *WebSocketServer) trackConnection(c *Connection) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if s.connections == nil {
		s.connections = make(map[*Connection]struct{})
	}
	s.connections[c] = struct{}{}
	return true
}

func (s *WebSocketServer) remove(c *Connection) {
	c.close()
	s.mu.Lock()
	delete(s.connections, c)
	s.mu.Unlock()
}

// Close 拒绝新连接，取消现有连接并等候请求退出；已接受 Run 由 Runner 管理。
func (s *WebSocketServer) Close() error {
	s.mu.Lock()
	s.closed = true
	for c := range s.connections {
		c.cancel()
	}
	httpServer, done := s.httpServer, s.serveDone
	s.mu.Unlock()
	var err error
	if httpServer != nil {
		err = httpServer.Close()
	}
	s.active.Wait()
	if done == nil {
		return err
	}
	// Close 可重复调用；Serve 的结束错误由第一次关闭者读取。
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.serveDone == nil {
		return err
	}
	serveErr := <-done
	s.serveDone = nil
	if errors.Is(serveErr, http.ErrServerClosed) {
		return err
	}
	return errors.Join(err, serveErr)
}

// 一条客户端连接的网络收发与请求循环，由 WebSocketServer 接入后创建。
type websocketClientConnection struct {
	// 当前连接与请求处理依赖。
	connection *Connection
	socket     *websocket.Conn
	rpc        *RPCServer

	// 读协程投递消息；退出时等待读写协程结束。
	incoming chan []byte
	workers  sync.WaitGroup
}

func (p *websocketClientConnection) run() {
	p.workers.Add(2)
	go p.read()
	go p.write()
	p.processRequests()

	// 返回前先取消连接，再等待两个读写协程收尾。
	p.connection.cancel()
	p.workers.Wait()
}

// processRequests 顺序交办当前连接的请求，读写协程持续独立工作。
func (p *websocketClientConnection) processRequests() {
	ctx := context.WithValue(p.connection.ctx, connectionKey{}, p.connection)
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case raw := <-p.incoming:
			response := p.rpc.Handle(ctx, raw)
			if response != nil {
				p.connection.enqueue(response)
			}
			// 响应先入队，再放行订阅期间缓冲的通知。
			p.connection.releaseBufferedNotifications()
		}
	}
}

func (p *websocketClientConnection) read() {
	defer p.workers.Done()
	defer p.connection.cancel()
	for {
		kind, raw, err := p.socket.Read(p.connection.ctx)
		if err != nil || kind != websocket.MessageText {
			return
		}
		select {
		case p.incoming <- raw:
		default:
			return
		}
	}
}

func (p *websocketClientConnection) write() {
	defer p.workers.Done()
	defer p.connection.cancel()
	for {
		select {
		case <-p.connection.ctx.Done():
			return
		case raw := <-p.connection.outgoing:
			ctx, cancel := context.WithTimeout(p.connection.ctx, 5*time.Second)
			err := p.socket.Write(ctx, websocket.MessageText, raw)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
