package rpc

import (
	"context"
	"sync"
	"sync/atomic"
)

const (
	callWaiting uint32 = iota
	callStarted
	callAbandoned
)

// 活对象。Scheduler 只让同一 Session 的写请求依次执行；不同 Session 互不等待。
type Scheduler struct {
	mu     sync.Mutex
	queues map[string]*sessionQueue
}

type sessionQueue struct {
	calls []*scheduledCall
}

type scheduledCall struct {
	ctx  context.Context
	call func() error
	done chan error

	state atomic.Uint32
}

func newScheduler() *Scheduler {
	return &Scheduler{queues: make(map[string]*sessionQueue)}
}

// Schedule 立即把调用接到对应 Session 队尾，并返回等待结果的函数。
// 排队时取消的请求不会进入业务处理。
func (s *Scheduler) Schedule(ctx context.Context, sessionID string, call func() error) func() error {
	item := &scheduledCall{
		ctx:  ctx,
		call: call,
		done: make(chan error, 1),
	}

	s.mu.Lock()
	queue := s.queues[sessionID]
	if queue == nil {
		queue = &sessionQueue{}
		s.queues[sessionID] = queue
	}
	queue.calls = append(queue.calls, item)
	if len(queue.calls) == 1 {
		go s.run(sessionID, queue)
	}
	s.mu.Unlock()

	return func() error {
		select {
		case err := <-item.done:
			return err
		case <-ctx.Done():
			if item.state.CompareAndSwap(callWaiting, callAbandoned) {
				return ctx.Err()
			}
			// 业务已经开始，必须等它按 Context 正常收尾；否则连接关闭会漏掉活 Handler。
			return <-item.done
		}
	}
}

func (s *Scheduler) run(sessionID string, queue *sessionQueue) {
	for {
		s.mu.Lock()
		if len(queue.calls) == 0 {
			delete(s.queues, sessionID)
			s.mu.Unlock()
			return
		}
		item := queue.calls[0]
		s.mu.Unlock()

		err := item.ctx.Err()
		if err == nil && item.state.CompareAndSwap(callWaiting, callStarted) {
			err = item.call()
		}
		item.done <- err

		s.mu.Lock()
		queue.calls[0] = nil
		queue.calls = queue.calls[1:]
		s.mu.Unlock()
	}
}
