package harness

import (
	"context"
	"testing"
	"time"

	"harness/kernel/events"
	"harness/kernel/runner"
	"harness/kernel/session"
)

func TestSnapshotAfterEndPublicationDoesNotResurrectRun(t *testing.T) {
	f := newTestFixture(t)
	defer f.host.Close()
	info, err := f.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	observed := make(chan Snapshot, 1)
	unlisten, err := events.Subscribe(f.events, func(_ context.Context, event runner.RunEvent) error {
		if event.Kind != runner.RunEnded {
			return nil
		}
		// 精确停在结束通知已发出、live 尚未释放的窗口，模拟此时才来订阅的 Client。
		snapshot, snapshotErr := f.service.Snapshot(event.SessionID)
		observed <- snapshot
		return snapshotErr
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unlisten()
	err = f.service.Start(context.Background(), RunInput{SessionID: info.Meta.ID, Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high", Message: session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "finish"}}}})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	f.loop.release()
	select {
	case snapshot := <-observed:
		if len(snapshot.Runs) != 0 {
			t.Fatalf("finished run reappeared: %+v", snapshot.Runs)
		}
		if len(snapshot.Entries) != 2 {
			t.Fatalf("missing durable result: %+v", snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("no end event")
	}
}
