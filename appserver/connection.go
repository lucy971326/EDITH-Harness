package appserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

const queueLimit = 128

type connectionKey struct{}

// 活对象。一个 Client 的协议状态、事件订阅和断线清理；不参与产品业务。
type Connection struct {
	// JSON-RPC 连接与生命周期。
	server *Server
	rpc    *jsonrpc2.Conn
	ctx    context.Context
	cancel context.CancelFunc

	// 协议初始化。
	initialized     atomic.Bool
	initializeTimer *time.Timer

	// 运行事件异步发给 Client，不能反压 Runner。
	notifications     chan notification
	notificationsDone chan struct{}

	// 请求保持接收顺序；断线时等待已经收到的请求退出。
	handling     sync.Mutex
	requests     sync.WaitGroup
	disconnected bool

	// 当前 Client 的订阅和断线状态。
	mu            sync.Mutex
	subscriptions map[string]*Subscription
}

type notification struct {
	method string
	params any
}

type subscriptionEvent struct {
	SubscriptionID string `json:"subscriptionID"`
	Event          any    `json:"event"`
}

func newConnection(parent context.Context, stream jsonrpc2.ObjectStream, server *Server) *Connection {
	ctx, cancel := context.WithCancel(parent)
	connection := &Connection{
		server:            server,
		ctx:               ctx,
		cancel:            cancel,
		notifications:     make(chan notification, queueLimit),
		notificationsDone: make(chan struct{}),
		subscriptions:     make(map[string]*Subscription),
	}
	connection.initializeTimer = time.AfterFunc(10*time.Second, cancel)
	connection.rpc = jsonrpc2.NewConn(ctx, stream, connection, jsonrpc2.SetLogger(discardLogger{}))
	return connection
}

func (c *Connection) run() {
	go c.sendNotifications()
	<-c.rpc.DisconnectNotify()

	c.mu.Lock()
	c.disconnected = true
	c.mu.Unlock()
	c.cancel()
	c.initializeTimer.Stop()
	c.requests.Wait()
	<-c.notificationsDone
	c.closeSubscriptions()
}

func (c *Connection) disconnect() {
	c.cancel()
}

// Handle 让协议库继续读取断线；请求本身仍按到达顺序处理。
func (c *Connection) Handle(ctx context.Context, rpc *jsonrpc2.Conn, request *jsonrpc2.Request) {
	c.mu.Lock()
	if c.disconnected {
		c.mu.Unlock()
		return
	}
	c.requests.Add(1)
	c.mu.Unlock()

	go func() {
		defer c.requests.Done()
		c.handling.Lock()
		defer c.handling.Unlock()
		c.handleRequest(ctx, rpc, request)
	}()
}

// handleRequest 写完响应后，才放行订阅期间缓存的通知。
func (c *Connection) handleRequest(ctx context.Context, rpc *jsonrpc2.Conn, request *jsonrpc2.Request) {
	ctx = context.WithValue(ctx, connectionKey{}, c)
	result, err := c.call(ctx, request)

	if !request.Notif {
		if err == nil {
			err = rpc.Reply(ctx, request.ID, result)
		} else {
			err = rpc.ReplyWithError(ctx, request.ID, rpcError(err))
		}
		if err != nil {
			c.cancel()
		}
	}

	c.releaseBufferedNotifications()
}

func (c *Connection) call(ctx context.Context, request *jsonrpc2.Request) (any, error) {
	params := json.RawMessage(`{}`)
	if request.Params != nil {
		params = *request.Params
	}

	if !c.initialized.Load() {
		if request.Method != "initialize" || request.Notif {
			return nil, &jsonrpc2.Error{Code: -32001, Message: "Initialize first"}
		}
		var input InitializeParams
		err := decodeParams(params, &input)
		if err != nil || input.ProtocolVersion != 1 {
			return nil, &jsonrpc2.Error{Code: -32001, Message: "Initialization rejected"}
		}
		c.initialized.Store(true)
		c.initializeTimer.Stop()
		return InitializeResult{ProtocolVersion: 1}, nil
	}

	if request.Method == "initialize" {
		return nil, &jsonrpc2.Error{Code: -32002, Message: "Connection already initialized"}
	}
	if request.Method == "server/unsubscribe" {
		var input struct {
			SubscriptionID string `json:"subscriptionID"`
		}
		err := decodeParams(params, &input)
		if err != nil || input.SubscriptionID == "" {
			return nil, &Error{Code: CodeInvalidParams, Message: "invalid subscription"}
		}
		c.unsubscribe(input.SubscriptionID)
		return struct{}{}, nil
	}

	return c.server.Call(ctx, request.Method, params)
}

func (c *Connection) sendNotifications() {
	defer close(c.notificationsDone)
	for {
		select {
		case <-c.ctx.Done():
			return
		case item := <-c.notifications:
			err := c.rpc.Notify(c.ctx, item.method, item.params)
			if err != nil {
				c.cancel()
				return
			}
		}
	}
}

func (c *Connection) enqueueNotification(item notification) {
	select {
	case <-c.ctx.Done():
		return
	case c.notifications <- item:
	default:
		// 慢 Client 直接断线，不能阻塞 Runner 或静默丢失完成事件。
		c.cancel()
	}
}

// ConnectionFrom 只供订阅处理方法取得当前 Client 的连接能力。
func ConnectionFrom(ctx context.Context) (*Connection, error) {
	connection, ok := ctx.Value(connectionKey{}).(*Connection)
	if !ok {
		return nil, &Error{Code: CodeConflict, Message: "connection required"}
	}
	return connection, nil
}

// Subscribe 创建暂存事件的订阅；请求响应写完后才开放通知发送。
func (c *Connection) Subscribe() (*Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx.Err() != nil {
		return nil, &Error{Code: CodeConflict, Message: "connection closed"}
	}
	subscription := &Subscription{ID: rand.Text(), connection: c}
	c.subscriptions[subscription.ID] = subscription
	return subscription, nil
}

func (c *Connection) releaseBufferedNotifications() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, subscription := range c.subscriptions {
		subscription.activate()
	}
}

func (c *Connection) unsubscribe(id string) {
	c.mu.Lock()
	subscription := c.subscriptions[id]
	c.mu.Unlock()
	if subscription != nil {
		subscription.Close()
	}
}

func (c *Connection) closeSubscriptions() {
	c.mu.Lock()
	subscriptions := c.subscriptions
	c.subscriptions = make(map[string]*Subscription)
	c.mu.Unlock()
	for _, subscription := range subscriptions {
		subscription.Close()
	}
}

// 活对象。连接拥有的一条临时事件订阅，关闭时解除业务事件监听。
type Subscription struct {
	ID         string
	connection *Connection

	mu      sync.Mutex
	active  bool
	closed  bool
	pending []notification
	cleanup func()
}

// SetCleanup 安装事件注销函数；连接若已断开则立即注销。
func (s *Subscription) SetCleanup(cleanup func()) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cleanup()
		return
	}
	s.cleanup = cleanup
	s.mu.Unlock()
}

// Notify 非阻塞地排队通知；积压超限时断开慢 Client。
func (s *Subscription) Notify(method string, event any) {
	item := notification{
		method: method,
		params: subscriptionEvent{SubscriptionID: s.ID, Event: event},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.active {
		s.connection.enqueueNotification(item)
		return
	}
	if len(s.pending) >= queueLimit {
		s.connection.cancel()
		return
	}
	s.pending = append(s.pending, item)
}

func (s *Subscription) activate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.active {
		return
	}
	s.active = true
	for _, item := range s.pending {
		s.connection.enqueueNotification(item)
	}
	s.pending = nil
}

// Close 幂等解除监听；不取消产生事件的任务。
func (s *Subscription) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	cleanup := s.cleanup
	s.pending = nil
	s.mu.Unlock()
	if cleanup != nil {
		cleanup()
	}

	s.connection.mu.Lock()
	delete(s.connection.subscriptions, s.ID)
	s.connection.mu.Unlock()
}

func decodeParams(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func rpcError(err error) *jsonrpc2.Error {
	var protocol *jsonrpc2.Error
	if errors.As(err, &protocol) {
		return protocol
	}

	var public *Error
	if !errors.As(err, &public) {
		return &jsonrpc2.Error{Code: jsonrpc2.CodeInternalError, Message: "Internal error"}
	}

	code := int64(jsonrpc2.CodeInternalError)
	switch public.Code {
	case CodeUnknownMethod:
		code = jsonrpc2.CodeMethodNotFound
	case CodeInvalidParams:
		code = jsonrpc2.CodeInvalidParams
	case CodeNotFound:
		code = -32004
	case CodeConflict:
		code = -32009
	}
	message := public.Message
	if code == jsonrpc2.CodeInternalError {
		message = "Internal error"
	}
	rpcError := &jsonrpc2.Error{Code: code, Message: message}
	rpcError.SetError(public.Code)
	return rpcError
}

type discardLogger struct{}

func (discardLogger) Printf(string, ...any) {}
