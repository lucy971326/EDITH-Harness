package subagents

import (
	"context"
	"errors"
	"testing"
	"time"

	"harness/kernel/session"
)

func TestStopFamilyDoesNotWaitForChildSettingsOrStartLateChild(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	handle, parent := notificationParent(t, f)
	entered := make(chan struct{})
	release := make(chan struct{})
	f.subagents.settings = &barrierSettingsStore{SessionSettingsStore: f.settings, beforePut: func(string) {
		close(entered)
		<-release
	}}
	spawnDone := make(chan error, 1)
	go func() {
		_, err := f.subagents.Spawn(context.Background(), SpawnInput{ParentSessionID: parent.SessionID, ParentRunID: parent.RunID, Description: "blocked startup"})
		spawnDone <- err
	}()
	awaitSignal(t, entered)
	stopDone := make(chan error, 1)
	go func() { stopDone <- f.subagents.StopFamily(context.Background(), parent.SessionID) }()
	stopped := false
	select {
	case err := <-stopDone:
		stopped = true
		if err != nil {
			t.Error(err)
		}
	case <-time.After(time.Second):
		t.Error("stop blocked behind child startup instead of cancelling parent")
	}
	close(release)
	err := <-spawnDone
	if err == nil {
		t.Error("child started after family stop")
	}
	if !stopped {
		<-stopDone
	}
	select {
	case <-handle.Done():
	case <-time.After(time.Second):
		t.Fatal("parent did not stop")
	}
	select {
	case <-f.loop.started:
		t.Fatal("stopped startup executed child Loop")
	default:
	}
}

func TestStoppedSendCannotCrossIntoNewParentRun(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	handle, parent := notificationParent(t, f)
	child := notificationChild(t, f, parent)
	f.loop.release()
	_, err := f.subagents.Wait(context.Background(), parent.SessionID, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	f.subagents.settings = &controlledSettings{SessionSettingsStore: f.settings, read: func() { close(entered); <-release }}
	done := make(chan error, 1)
	go func() {
		_, err := f.subagents.Send(context.Background(), parent.SessionID, parent.RunID, child.TaskID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "stale instruction"}}})
		done <- err
	}()
	awaitSignal(t, entered)
	err = f.subagents.StopFamily(context.Background(), parent.SessionID)
	if err != nil {
		t.Error(err)
	}
	awaitSignal(t, handle.Done())
	next, err := f.runner.Start(context.Background(), parent.SessionID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "new user request"}}})
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	err = <-done
	if !errors.Is(err, ErrFamilyStopped) {
		t.Fatalf("stale send crossed stop: %v", err)
	}
	f.subagents.settings = f.settings
	<-f.loop.parentInvocations
	_, err = f.subagents.Send(context.Background(), parent.SessionID, parent.RunID, child.TaskID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "old run retry"}}})
	if !errors.Is(err, ErrFamilyStopped) {
		t.Fatalf("old run re-admitted: %v", err)
	}
	sent, err := f.subagents.Send(context.Background(), parent.SessionID, next.RunID(), child.TaskID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "explicit new instruction"}}})
	if err != nil || sent.Turn != 2 {
		t.Fatalf("new run could not resume child: %+v, %v", sent, err)
	}
	f.loop.waitStarted(t)
}

func TestSingleChildStopInvalidatesPendingSend(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	_, parent := notificationParent(t, f)
	child := notificationChild(t, f, parent)
	f.loop.release()
	_, err := f.subagents.Wait(context.Background(), parent.SessionID, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	f.subagents.settings = &controlledSettings{SessionSettingsStore: f.settings, read: func() { close(entered); <-release }}
	done := make(chan error, 1)
	go func() {
		_, err := f.subagents.Send(context.Background(), parent.SessionID, parent.RunID, child.TaskID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "before stop"}}})
		done <- err
	}()
	awaitSignal(t, entered)
	err = f.subagents.Stop(context.Background(), parent.SessionID, child.TaskID)
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	err = <-done
	if !errors.Is(err, ErrTaskStopped) {
		t.Fatalf("pending send resumed stopped child: %v", err)
	}
	f.subagents.settings = f.settings
	_, err = f.subagents.Send(context.Background(), parent.SessionID, parent.RunID, child.TaskID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "after stop"}}})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
}
