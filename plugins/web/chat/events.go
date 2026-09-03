package chat

import (
	"sync"

	"harness/kernel/runner"
)

// 数据。一个等待写入页面 SSE 连接的通知。
type pageEvent struct {
	run  *runner.RunEvent
	dock *DockChanged
}

func pageRunEvent(event runner.RunEvent) pageEvent {
	return pageEvent{run: &event}
}

func pageDockEvent(event DockChanged) pageEvent {
	return pageEvent{dock: &event}
}

// 活对象。按会话把页面通知转给当前 SSE 连接的内存队列。
type eventHub struct {
	mu      sync.Mutex
	clients map[string]map[chan pageEvent]struct{}
	closed  bool
}

func newEventHub() *eventHub {
	return &eventHub{clients: make(map[string]map[chan pageEvent]struct{})}
}

func (h *eventHub) subscribe(sessionID string) (<-chan pageEvent, func()) {
	ch := make(chan pageEvent, 64)
	h.mu.Lock()
	if h.closed {
		close(ch)
	} else {
		if h.clients[sessionID] == nil {
			h.clients[sessionID] = make(map[chan pageEvent]struct{})
		}
		h.clients[sessionID][ch] = struct{}{}
	}
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() { h.remove(sessionID, ch) })
	}
}

func (h *eventHub) publish(sessionID string, event pageEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients[sessionID] {
		select {
		case ch <- event:
		default:
			delete(h.clients[sessionID], ch)
			close(ch)
		}
	}
	if len(h.clients[sessionID]) == 0 {
		delete(h.clients, sessionID)
	}
}

func (h *eventHub) publishRun(event runner.RunEvent) {
	h.publish(event.SessionID, pageRunEvent(event))
}

func (h *eventHub) publishDock(event DockChanged) {
	h.publish(event.SessionID, pageDockEvent(event))
}

func (h *eventHub) remove(sessionID string, ch chan pageEvent) {
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
