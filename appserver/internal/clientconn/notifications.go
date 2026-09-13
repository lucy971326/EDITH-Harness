package clientconn

import (
	"crypto/rand"
	"sync"

	internalrpc "harness/appserver/internal/rpc"
)

const (
	notificationQueueLimit   = 128
	subscriptionPendingLimit = 128
)

type notification struct {
	method string
	params any
}

type subscriptionEvent struct {
	SubscriptionID string `json:"subscriptionID"`
	Event          any    `json:"event"`
}

type subscriptionSet struct {
	mu    sync.Mutex
	items map[string]*Subscription
}

func newSubscriptionSet() subscriptionSet {
	return subscriptionSet{items: make(map[string]*Subscription)}
}

func (s *subscriptionSet) create(connection *Connection) (*Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if connection.ctx.Err() != nil {
		return nil, &internalrpc.Error{Code: internalrpc.CodeConflict, Message: "connection closed"}
	}
	subscription := &Subscription{id: rand.Text(), connection: connection, owner: s}
	s.items[subscription.id] = subscription
	return subscription, nil
}

func (s *subscriptionSet) close(id string) {
	s.mu.Lock()
	subscription := s.items[id]
	s.mu.Unlock()
	if subscription != nil {
		subscription.Close()
	}
}

func (s *subscriptionSet) remove(id string) {
	s.mu.Lock()
	delete(s.items, id)
	s.mu.Unlock()
}

func (s *subscriptionSet) closeAll() {
	s.mu.Lock()
	items := s.items
	s.items = make(map[string]*Subscription)
	s.mu.Unlock()
	for _, subscription := range items {
		subscription.close(false)
	}
}

// 活对象。Subscription 是一个 Client 拥有的临时事件订阅；关闭只解除监听，不停止 Run。
type Subscription struct {
	id         string
	connection *Connection
	owner      *subscriptionSet

	mu      sync.Mutex
	active  bool
	closed  bool
	pending []notification
	cleanup func()
}

func (s *Subscription) ID() string { return s.id }

// Done 在 Client 断开时关闭。
func (s *Subscription) Done() <-chan struct{} { return s.connection.ctx.Done() }

// Disconnect 断开无法继续可靠接收事件的 Client。
func (s *Subscription) Disconnect() { s.connection.disconnect() }

// SetCleanup 安装事件注销函数；订阅若已关闭则立即注销。
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
		params: subscriptionEvent{SubscriptionID: s.id, Event: event},
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
	if len(s.pending) >= subscriptionPendingLimit {
		s.connection.disconnect()
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

// Close 幂等解除监听。
func (s *Subscription) Close() {
	s.close(true)
}

func (s *Subscription) close(remove bool) {
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
	if remove {
		s.owner.remove(s.id)
	}
}

// 活对象。RequestContext 收集当前请求创建的订阅，确保只在本请求响应后放行。
type RequestContext struct {
	connection    *Connection
	subscriptions []*Subscription
}

// Subscribe 创建一条先缓冲事件的订阅。
func (r *RequestContext) Subscribe() (*Subscription, error) {
	subscription, err := r.connection.subscriptions.create(r.connection)
	if err != nil {
		return nil, err
	}
	r.subscriptions = append(r.subscriptions, subscription)
	return subscription, nil
}

func (r *RequestContext) activateSubscriptions() {
	for _, subscription := range r.subscriptions {
		subscription.activate()
	}
}

func (r *RequestContext) closeSubscriptions() {
	for _, subscription := range r.subscriptions {
		subscription.Close()
	}
}
