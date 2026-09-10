package appserver

import (
	"context"
	"crypto/rand"
	"sync"
	"sync/atomic"
	"time"
)

const queueLimit = 128

type connectionKey struct{}

// 活对象。一条 Client 连接的取消、发送队列和订阅；不拥有后台 Run。
type Connection struct {
	// 连接生命周期与协议握手。
	ctx             context.Context
	cancel          context.CancelFunc
	initialized     atomic.Bool
	initializeTimer *time.Timer

	// 串行发送队列，由 WebSocket 写协程消费。
	outgoing chan []byte

	// 当前连接拥有的订阅；mu 保护登记与移除。
	mu            sync.Mutex
	subscriptions map[string]*Subscription
}

// ConnectionFrom 只供需要连接能力的处理方法取用；普通业务方法不需要它。
func ConnectionFrom(ctx context.Context) (*Connection, error) {
	c, ok := ctx.Value(connectionKey{}).(*Connection)
	if !ok {
		return nil, &Error{Code: CodeConflict, Message: "connection required"}
	}
	return c, nil
}

// Subscribe 创建暂存事件的订阅；请求响应入队后才开放通知发送。
func (c *Connection) Subscribe() (*Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx.Err() != nil || len(c.subscriptions) >= 32 {
		return nil, &Error{Code: CodeConflict, Message: "connection closed or subscription limit reached"}
	}
	s := &Subscription{ID: rand.Text(), connection: c}
	c.subscriptions[s.ID] = s
	return s, nil
}

func (c *Connection) enqueue(raw []byte) {
	if c.ctx.Err() != nil {
		return
	}
	select {
	case c.outgoing <- raw:
	default:
		// 断开慢连接，不让 Runner 等待网络，也不悄悄丢掉完成事件。
		c.cancel()
	}
}

func (c *Connection) releaseBufferedNotifications() {
	c.mu.Lock()
	items := make([]*Subscription, 0, len(c.subscriptions))
	for _, item := range c.subscriptions {
		items = append(items, item)
	}
	c.mu.Unlock()
	for _, item := range items {
		item.activate()
	}
}

func (c *Connection) unsubscribe(id string) {
	c.mu.Lock()
	s := c.subscriptions[id]
	c.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

func (c *Connection) close() {
	c.cancel()
	c.initializeTimer.Stop()
	c.mu.Lock()
	items := c.subscriptions
	c.subscriptions = make(map[string]*Subscription)
	c.mu.Unlock()
	for _, item := range items {
		item.Close()
	}
}

// 活对象。连接拥有的一条临时事件订阅，关闭时解除业务事件监听。
type Subscription struct {
	// 订阅身份与所属连接。
	ID         string
	connection *Connection

	// 通知缓冲与关闭状态，由 mu 保护。
	mu      sync.Mutex
	active  bool
	closed  bool
	pending [][]byte

	// 关闭时解除产品事件监听。
	cleanup func()
}

// SetCleanup 安装事件注销函数；处理连接在组装订阅时已经断开的情况。
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

// Notify 只做有界内存入队，不等待 Client；数据无法编码时关闭连接。
func (s *Subscription) Notify(method string, event any) {
	raw, err := encodeNotification(method, s.ID, event)
	if err != nil {
		s.connection.cancel()
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.active {
		s.connection.enqueue(raw)
		return
	}
	if len(s.pending) >= queueLimit {
		s.connection.cancel()
		return
	}
	s.pending = append(s.pending, raw)
}

func (s *Subscription) activate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.active {
		return
	}
	s.active = true
	for _, raw := range s.pending {
		s.connection.enqueue(raw)
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
