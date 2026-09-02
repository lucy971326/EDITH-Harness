package chat

import (
	"sync"

	"harness/kernel/runner"
)

// 活对象。按会话把 Runner 事件转给当前 SSE 连接的内存队列。
type eventHub struct {
	mu      sync.Mutex
	clients map[string]map[chan runner.RunEvent]struct{}
	closed  bool
}

func newEventHub() *eventHub {
	return &eventHub{clients: make(map[string]map[chan runner.RunEvent]struct{})}
}

func (h *eventHub) subscribe(sessionID string) (<-chan runner.RunEvent, func()) {
	ch := make(chan runner.RunEvent, 64)
	h.mu.Lock()
	if h.closed {
		close(ch)
	} else {
		if h.clients[sessionID] == nil {
			h.clients[sessionID] = make(map[chan runner.RunEvent]struct{})
		}
		h.clients[sessionID][ch] = struct{}{}
	}
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() { h.remove(sessionID, ch) })
	}
}

func (h *eventHub) publish(event runner.RunEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients[event.SessionID] {
		select {
		case ch <- event:
		default:
			delete(h.clients[event.SessionID], ch)
			close(ch)
		}
	}
	if len(h.clients[event.SessionID]) == 0 {
		delete(h.clients, event.SessionID)
	}
}

func (h *eventHub) remove(sessionID string, ch chan runner.RunEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := h.clients[sessionID]
	if _, ok := clients[ch]; !ok {
		return
	}
	delete(clients, ch)
	close(ch)
	if len(clients) == 0 {
		delete(h.clients, sessionID)
	}
}

func (h *eventHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for sessionID, clients := range h.clients {
		for ch := range clients {
			close(ch)
		}
		delete(h.clients, sessionID)
	}
}
