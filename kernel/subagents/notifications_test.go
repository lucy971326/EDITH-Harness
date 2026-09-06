package subagents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/loops"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

func notificationParent(t *testing.T, f subagentsFixture) (*runner.RunHandle, loops.Invocation) {
	t.Helper()
	_, err := f.sessions.Create("parent-session")
	if err != nil {
		t.Fatal(err)
	}
	err = f.settings.Put("parent-session", settings.SessionSettings{AgentID: agents.DefaultID, Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high", Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := f.runner.Start(context.Background(), "parent-session", session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "parent request"}}})
	if err != nil {
		t.Fatal(err)
	}
	return handle, <-f.loop.parentInvocations
}

func notificationChild(t *testing.T, f subagentsFixture, parent loops.Invocation) SpawnResult {
	t.Helper()
	child, err := f.subagents.Spawn(context.Background(), SpawnInput{ParentSessionID: parent.SessionID, ParentRunID: parent.RunID, Description: "child request"})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	return child
}

func collaborationEntries(t *testing.T, f subagentsFixture, parent string) []session.Entry {
	t.Helper()
	sess, err := f.sessions.Get(parent)
	if err != nil {
		t.Fatal(err)
	}
	var entries []session.Entry
	for _, entry := range sess.Entries() {
		if entry.Message.Role == session.RoleCollaboration {
			entries = append(entries, entry)
		}
	}
	return entries
}

func TestCompletedChildrenAutomaticallyNotifyOnce(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	_, parent := notificationParent(t, f)
	first := notificationChild(t, f, parent)
	second := notificationChild(t, f, parent)
	arrived := make(chan session.Message, 4)
	_, err := events.Subscribe(f.events, func(_ context.Context, event runner.RunEvent) error {
		if event.Entry != nil && event.Entry.Message.Role == session.RoleCollaboration {
			arrived <- event.Entry.Message
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.release()
	f.loop.release()
	for i := 0; i < 2; i++ {
		select {
		case message := <-arrived:
			if message.SourceRunID == "" || !strings.Contains(message.Blocks[0].Text, "child assistant answer") {
				t.Fatalf("wrong result: %+v", message)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("automatic notification missing")
		}
	}
	response, err := f.subagents.Wait(context.Background(), parent.SessionID, WaitInput{TaskIDs: []string{first.TaskID, second.TaskID}, Timeout: time.Second})
	if err != nil || response.Reason != "completed" || len(response.Notifications) != 2 {
		t.Fatalf("wait: %+v, %v", response, err)
	}
	entries := collaborationEntries(t, f, parent.SessionID)
	if len(entries) != 2 {
		t.Fatalf("wait duplicated body: %+v", entries)
	}
	messages, err := parent.Checkpoint(context.Background(), loops.CheckpointContinue)
	if err != nil || len(messages) != 2 {
		t.Fatalf("checkpoint: %+v, %v", messages, err)
	}
	var seen []string
	for _, item := range response.Notifications {
		seen = append(seen, item.NotificationID)
	}
	response, err = f.subagents.Wait(context.Background(), parent.SessionID, WaitInput{TaskIDs: []string{first.TaskID, second.TaskID}, SeenNotificationIDs: seen, Timeout: 10 * time.Millisecond, InputSignal: parent.InputSignal()})
	if err != nil || response.Reason != "timeout" || len(response.Notifications) != 0 {
		t.Fatalf("seen completion woke wait: %+v, %v", response, err)
	}
}

func TestFinalCheckpointRetainsNotificationUntilUserStarts(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	handle, parent := notificationParent(t, f)
	child := notificationChild(t, f, parent)
	messages, err := parent.Checkpoint(context.Background(), loops.CheckpointFinal)
	if err != nil || len(messages) != 0 {
		t.Fatalf("final checkpoint: %+v, %v", messages, err)
	}
	f.loop.release()
	_, err = f.subagents.Wait(context.Background(), parent.SessionID, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(collaborationEntries(t, f, parent.SessionID)) != 0 {
		t.Fatal("notification crossed final checkpoint")
	}
	f.loop.releaseParent()
	if handle.Wait().Err != nil {
		t.Fatal("parent failed")
	}
	err = f.subagents.deliver(parent.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := f.runner.State(parent.SessionID); active {
		t.Fatal("notification automatically started idle parent")
	}
	task, err := f.subagents.store.loadTask(child.TaskID)
	if err != nil || task.Notifications[0].Delivered {
		t.Fatalf("pending notification lost: %+v, %v", task, err)
	}
	// 模拟服务重启：只恢复记录，直到用户明确启动父会话才接收。
	err = f.subagents.Close()
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := NewSubagents(f.sessions, f.settings, f.subagents.agents, f.subagents.models, f.runner, f.events, filepath.Dir(f.subagents.store.dir))
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if _, active := f.runner.State(parent.SessionID); active {
		t.Fatal("recovery started parent")
	}
	next, err := f.runner.Start(context.Background(), parent.SessionID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "continue"}}})
	if err != nil {
		t.Fatal(err)
	}
	invocation := <-f.loop.parentInvocations
	count := 0
	for _, message := range invocation.History {
		if message.Role == session.RoleCollaboration {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("initial history lacks pending notification: %+v", invocation.History)
	}
	messages, err = invocation.Checkpoint(context.Background(), loops.CheckpointFinal)
	if err != nil || len(messages) != 0 {
		t.Fatalf("startup notification repeated: %+v, %v", messages, err)
	}
	f.loop.releaseParent()
	if next.Wait().Err != nil {
		t.Fatal("next parent run failed")
	}
}

func TestNotificationIdentityMustMatchStoredTurn(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	_, parent := notificationParent(t, f)
	child := notificationChild(t, f, parent)
	f.loop.release()
	_, err := f.subagents.Wait(context.Background(), parent.SessionID, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.subagents.store.loadTask(child.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Task){
		func(task *Task) { task.Notifications[0].NotificationID = "other" },
		func(task *Task) { task.Notifications[0].ChildSessionID = "other" },
		func(task *Task) { task.Notifications[0].RunID = "other" },
		func(task *Task) { task.Notifications[0].Turn = 2 },
		func(task *Task) { task.Notifications[0].Status = StatusRunning },
		func(task *Task) { task.Notifications = append(task.Notifications, task.Notifications[0]) },
	} {
		broken := deepCopyTask(task)
		mutate(&broken)
		err = validateTask(broken, task.ID)
		if !errors.Is(err, ErrInvalidTaskData) {
			t.Fatalf("invalid notification accepted: %+v, %v", broken.Notifications, err)
		}
	}
}

func TestNotificationAcknowledgmentFailureRetriesWithoutDuplicate(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	_, parent := notificationParent(t, f)
	child := notificationChild(t, f, parent)
	blockedPath := filepath.Join(f.subagents.store.dir, child.TaskID+".json.tmp")
	var publications atomic.Int32
	_, err := events.Subscribe(f.events, func(_ context.Context, event runner.RunEvent) error {
		if event.Entry == nil || event.Entry.Message.Role != session.RoleCollaboration {
			return nil
		}
		publications.Add(1)
		return os.Mkdir(blockedPath, 0700)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.release()
	_, err = f.subagents.Wait(context.Background(), parent.SessionID, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err == nil {
		t.Fatal("acknowledgment save failure hidden")
	}
	tasks, listErr := f.subagents.List(parent.SessionID, child.TaskID)
	if listErr == nil || tasks[0].Notifications[0].Delivered {
		t.Fatalf("failed ack reported confirmed: %+v, %v", tasks, listErr)
	}
	err = os.Remove(blockedPath)
	if err != nil {
		t.Fatal(err)
	}
	err = f.subagents.deliver(parent.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if publications.Load() != 1 || len(collaborationEntries(t, f, parent.SessionID)) != 1 {
		t.Fatal("retry injected duplicate input")
	}
	task, err := f.subagents.store.loadTask(child.TaskID)
	if err != nil || !task.Notifications[0].Delivered {
		t.Fatalf("retry not confirmed: %+v, %v", task, err)
	}
	_, err = f.subagents.List(parent.SessionID, child.TaskID)
	if err != nil {
		t.Fatal("resolved delivery error remained", err)
	}
}

func TestWaitUserInputAndCancellationDoNotStopChildren(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	_, parent := notificationParent(t, f)
	child := notificationChild(t, f, parent)
	ctx := &observedWaitContext{Context: context.Background(), entered: make(chan struct{})}
	done := make(chan WaitResponse, 1)
	failed := make(chan error, 1)
	go func() {
		response, err := f.subagents.Wait(ctx, parent.SessionID, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second, InputSignal: parent.InputSignal()})
		done <- response
		failed <- err
	}()
	awaitSignal(t, ctx.entered)
	err := f.runner.Steer(parent.SessionID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "new request"}}})
	if err != nil {
		t.Fatal(err)
	}
	response := <-done
	err = <-failed
	if err != nil || response.Reason != "input" {
		t.Fatalf("steer did not wake wait: %+v, %v", response, err)
	}
	messages, err := parent.Checkpoint(context.Background(), loops.CheckpointContinue)
	if err != nil || len(messages) != 1 || messages[0].Blocks[0].Text != "new request" {
		t.Fatalf("wait consumed steer: %+v, %v", messages, err)
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = f.subagents.Wait(cancelCtx, parent.SessionID, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, active := f.runner.State(child.ChildSessionID); !active {
		t.Fatal("wait stopped child")
	}
}
