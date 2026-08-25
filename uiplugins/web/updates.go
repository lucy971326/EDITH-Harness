package web

import "sync"

// updateHub 把同步账本广播变成不阻塞的网页刷新提示。
type updateHub struct {
	mu          sync.Mutex
	subscribers map[string]map[chan updateNotice]struct{}
	closed      bool
}

// updateNotice 描述一轮账本变化需要刷新的网页区域。
type updateNotice struct {
	Chat     bool
	Composer bool
}

func newUpdateHub() *updateHub {
	return &updateHub{subscribers: make(map[string]map[chan updateNotice]struct{})}
}

// Subscribe 订阅一段会话的刷新提示；返回取消订阅函数。
func (h *updateHub) Subscribe(sessionID string) (<-chan updateNotice, func()) {
	updates := make(chan updateNotice, 1)
	h.mu.Lock()
	if h.closed {
		close(updates)
		h.mu.Unlock()
		return updates, func() {}
	}
	if h.subscribers[sessionID] == nil {
		h.subscribers[sessionID] = make(map[chan updateNotice]struct{})
	}
	h.subscribers[sessionID][updates] = struct{}{}
	h.mu.Unlock()

	unregister := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		listeners := h.subscribers[sessionID]
		if _, exists := listeners[updates]; !exists {
			return
		}
		delete(listeners, updates)
		close(updates)
		if len(listeners) == 0 {
			delete(h.subscribers, sessionID)
		}
	}
	return updates, unregister
}

// Publish 非阻塞通知一段会话的网页订阅者；慢客户端只会合并成一次刷新。
func (h *updateHub) Publish(sessionID string, notice updateNotice) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for updates := range h.subscribers[sessionID] {
		select {
		case updates <- notice:
		default:
			select {
			case pending := <-updates:
				pending.Chat = pending.Chat || notice.Chat
				pending.Composer = pending.Composer || notice.Composer
				updates <- pending
			default:
			}
		}
	}
}

// Close 关闭全部订阅，供 App 收摊。
func (h *updateHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for _, listeners := range h.subscribers {
		for updates := range listeners {
			close(updates)
		}
	}
	h.subscribers = nil
}
