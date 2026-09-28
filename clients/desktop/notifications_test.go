package desktop

import (
	"testing"

	"harness/internal/approvals"
	"harness/internal/runner"
)

func TestNotificationEventsDoNotBackfillOrReportCancellation(t *testing.T) {
	n := &Notifications{enabled: true, queue: make(chan notificationItem, 4), known: make(map[string]struct{})}
	old := approvals.Pending{ID: "old", Identity: approvals.Identity{SessionID: "session"}}
	newItem := approvals.Pending{ID: "new", Identity: approvals.Identity{SessionID: "session"}}
	n.onApprovals([]approvals.Pending{old}, true)
	if len(n.queue) != 0 {
		t.Fatal("initial approval snapshot generated a notification")
	}
	n.onApprovals([]approvals.Pending{old, newItem}, false)
	n.onApprovals([]approvals.Pending{old, newItem}, false)
	n.onRun(runner.RunEvent{SessionID: "session", RunID: "cancelled", Kind: runner.RunEnded, Status: runner.RunCancelled})
	if len(n.queue) != 1 {
		t.Fatalf("approval/duplicate/cancellation generated %d notifications, want 1", len(n.queue))
	}
	if item := <-n.queue; item.id != "approval-new" || item.sessionID != "session" {
		t.Fatalf("approval notification = %+v", item)
	}
	n.onRun(runner.RunEvent{SessionID: "session", RunID: "failed", Kind: runner.RunEnded, Status: runner.RunFailed})
	if item := <-n.queue; item.id != "run-failed" || item.body != "任务执行失败" {
		t.Fatalf("failed run notification = %+v", item)
	}
	n.enabled = false
	n.onRun(runner.RunEvent{SessionID: "session", RunID: "success", Kind: runner.RunEnded, Status: runner.RunSucceeded})
	if len(n.queue) != 0 {
		t.Fatal("disabled notification queue accepted a Run")
	}
}
