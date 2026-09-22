package approvals

import (
	"context"
	"errors"
	"testing"
	"time"

	"harness/kernel/permissions"
)

func TestApprovalLifecycle(t *testing.T) {
	for _, outcome := range []string{"approve", "reject", "cancel", "close"} {
		t.Run(outcome, func(t *testing.T) {
			service := New()
			defer service.Close()
			snapshot, updates, unsubscribe := service.Subscribe()
			if len(snapshot) != 0 {
				t.Fatal(snapshot)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			request := permissions.ApprovalRequest{ToolName: "exec_command", Arguments: []byte(`{"cmd":"test"}`), Requested: permissions.ExtraPermissions{Network: true}}
			result := make(chan error, 1)
			go func() {
				policy, err := service.Authorize(ctx, Identity{"session", "run", "call"}, permissions.HumanReviewer, request)
				if err == nil && (!policy.Network || request.Current.Network) {
					result <- errors.New("grant mutated baseline or missing network")
					return
				}
				result <- err
			}()
			var pending []Pending
			select {
			case pending = <-updates:
			case <-time.After(time.Second):
				t.Fatal("request not published")
			}
			if len(pending) != 1 {
				t.Fatal(pending)
			}
			id := pending[0].ID
			// 断线只解除监听；新订阅原子恢复原申请。
			unsubscribe()
			snapshot, updates, unsubscribe = service.Subscribe()
			defer unsubscribe()
			if len(snapshot) != 1 || snapshot[0].ID != id {
				t.Fatal(snapshot)
			}
			switch outcome {
			case "approve", "reject":
				err := service.Respond(id, permissions.Decision{Approved: outcome == "approve"})
				if err != nil {
					t.Fatal(err)
				}
			case "cancel":
				cancel()
			case "close":
				service.Close()
			}
			select {
			case err := <-result:
				if (outcome == "approve") != (err == nil) {
					t.Fatalf("outcome %s: %v", outcome, err)
				}
			case <-time.After(time.Second):
				t.Fatal("approval did not finish")
			}
			if !errors.Is(service.Respond(id, permissions.Decision{Approved: true}), ErrExpired) {
				t.Fatal("repeated answer accepted")
			}
			snapshot, _, stop := service.Subscribe()
			stop()
			if len(snapshot) != 0 {
				t.Fatal("completed request retained")
			}
		})
	}
}

func TestCancelledApprovalCannotGrant(t *testing.T) {
	service := New()
	defer service.Close()
	_, updates, unsubscribe := service.Subscribe()
	defer unsubscribe()
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan error, 1)
		go func() {
			_, err := service.Authorize(ctx, Identity{"s", "r", "t"}, permissions.HumanReviewer, permissions.ApprovalRequest{Requested: permissions.ExtraPermissions{Network: true}})
			result <- err
		}()
		var id string
		for id == "" {
			select {
			case snapshot := <-updates:
				if len(snapshot) != 0 {
					id = snapshot[0].ID
				}
			case <-time.After(time.Second):
				t.Fatal("missing request")
			}
		}
		cancel()
		if !errors.Is(service.Respond(id, permissions.Decision{Approved: true}), ErrExpired) {
			t.Fatal("cancelled answer accepted")
		}
		if !errors.Is(<-result, context.Canceled) {
			t.Fatal("cancelled request granted")
		}
	}
}
