package backend

import (
	"context"
	"sync"

	"harness/internal/approvals"
	"harness/internal/events"
	"harness/internal/runner"
)

// SubscribeActivity 提供进程内只读事件。审批初始快照不回放，取消时等待转发退出。
func (b *Backend) SubscribeActivity(onApprovals func([]approvals.Pending, bool), onRun func(runner.RunEvent)) (func(), error) {
	snapshot, updates, unsubscribeApprovals := b.approvals.Subscribe()
	unlistenRun, err := events.Subscribe(b.events, func(_ context.Context, event runner.RunEvent) error {
		onRun(event)
		return nil
	})
	if err != nil {
		unsubscribeApprovals()
		return nil, err
	}
	onApprovals(snapshot, true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for snapshot := range updates {
			onApprovals(snapshot, false)
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			unlistenRun()
			unsubscribeApprovals()
			<-done
		})
	}, nil
}
