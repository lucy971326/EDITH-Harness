package appserver

import (
	"context"
	"testing"

	"harness/kernel/events"
	"harness/kernel/runner"
	"harness/products/harness"
)

func newOrderTestListener(t *testing.T) *runListener {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	connection := &Connection{ctx: ctx, cancel: cancel, notifications: make(chan notification, queueLimit)}
	subscription := &Subscription{connection: connection, active: true}
	return &runListener{sessionID: "s", subscription: subscription, pending: make(map[uint64]runner.RunEvent)}
}

func TestRunListenerOrdersReentrantEvents(t *testing.T) {
	listener := newOrderTestListener(t)
	listener.start(harness.Snapshot{SeqEpoch: "epoch", UpdateSeq: 3})
	registry := events.NewRegistry()
	// 第一个监听在处理 4 时同步发出 5，模拟子任务协作回调重入。
	_, err := events.Subscribe(registry, func(ctx context.Context, event runner.RunEvent) error {
		if event.UpdateSeq == 4 {
			return events.Publish(ctx, registry, runner.RunEvent{SessionID: "s", SeqEpoch: "epoch", UpdateSeq: 5})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = events.Subscribe(registry, listener.receive)
	if err != nil {
		t.Fatal(err)
	}
	err = events.Publish(context.Background(), registry, runner.RunEvent{SessionID: "s", SeqEpoch: "epoch", UpdateSeq: 4})
	if err != nil {
		t.Fatal(err)
	}
	for _, seq := range []uint64{4, 5} {
		select {
		case item := <-listener.subscription.connection.notifications:
			event := item.params.(subscriptionEvent).Event.(runner.RunEvent)
			if event.UpdateSeq != seq {
				t.Fatalf("seq=%d, want %d", event.UpdateSeq, seq)
			}
		default:
			t.Fatalf("missing seq %d", seq)
		}
	}
}

func TestRunListenerSnapshotBoundaryAndDuplicate(t *testing.T) {
	listener := newOrderTestListener(t)
	for _, seq := range []uint64{6, 4, 5} {
		listener.receive(context.Background(), runner.RunEvent{SessionID: "s", SeqEpoch: "epoch", UpdateSeq: seq})
	}
	listener.start(harness.Snapshot{SeqEpoch: "epoch", UpdateSeq: 5})
	listener.receive(context.Background(), runner.RunEvent{SessionID: "s", SeqEpoch: "epoch", UpdateSeq: 6})
	if len(listener.subscription.connection.notifications) != 1 || listener.through != 6 || len(listener.pending) != 0 {
		t.Fatalf("boundary/duplicate mismatch: %+v", listener)
	}
}

func TestRunListenerBoundedGapAndEpoch(t *testing.T) {
	for _, epochChanged := range []bool{false, true} {
		listener := newOrderTestListener(t)
		listener.start(harness.Snapshot{SeqEpoch: "epoch"})
		if epochChanged {
			listener.receive(context.Background(), runner.RunEvent{SessionID: "s", SeqEpoch: "other", UpdateSeq: 1})
		} else {
			for seq := uint64(2); seq <= queueLimit+2; seq++ {
				listener.receive(context.Background(), runner.RunEvent{SessionID: "s", SeqEpoch: "epoch", UpdateSeq: seq})
			}
		}
		if listener.subscription.connection.ctx.Err() == nil {
			t.Fatal("invalid stream did not disconnect")
		}
	}
}
