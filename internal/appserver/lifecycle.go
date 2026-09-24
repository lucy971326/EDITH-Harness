package appserver

import (
	"context"
	"sync"
)

// serverLifecycle 统一管理工作准入、关闭广播和收尾等待。
type serverLifecycle struct {
	mu     sync.Mutex
	closed bool
	active sync.WaitGroup

	shutdownContext context.Context
	shutdown        context.CancelFunc
}

func newServerLifecycle() serverLifecycle {
	shutdownContext, shutdown := context.WithCancel(context.Background())
	return serverLifecycle{
		shutdownContext: shutdownContext,
		shutdown:        shutdown,
	}
}

// begin 登记一项新工作；关闭开始后不再接受。
func (l *serverLifecycle) begin() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	l.active.Add(1)
	return true
}

func (l *serverLifecycle) end() {
	l.active.Done()
}

// close 封闭入口，并通知所有 Client 连接断开。
func (l *serverLifecycle) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	l.shutdown()
}

func (l *serverLifecycle) wait() {
	l.active.Wait()
}

func (l *serverLifecycle) context() context.Context {
	return l.shutdownContext
}
