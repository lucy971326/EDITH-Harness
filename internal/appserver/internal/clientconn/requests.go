package clientconn

import "sync"

// connectionGate 只管理一个 Client 的请求准入与收尾，不决定业务请求顺序。
type connectionGate struct {
	mu     sync.Mutex
	closed bool
	active sync.WaitGroup
}

func (g *connectionGate) begin() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	g.active.Add(1)
	return true
}

func (g *connectionGate) end() {
	g.active.Done()
}

func (g *connectionGate) close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}

func (g *connectionGate) wait() {
	g.active.Wait()
}
