package subagents

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"harness/kernel/persist"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 活对象。通过已有设置存储契约复现读取阻塞。
type controlledSettings struct {
	settings.SessionSettingsStore
	read func()
}

// 活对象。确认 Wait 已进入等待，不靠调度延时猜测。
type observedWaitContext struct {
	context.Context
	once    sync.Once
	entered chan struct{}
}

func (c *observedWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

func (s *controlledSettings) For(id string) (settings.SessionSettings, error) {
	if s.read != nil {
		s.read()
	}
	return s.SessionSettingsStore.For(id)
}

func awaitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not finish")
	}
}

func TestCloseWhileSendReadsSettings(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	parent, run := createParentRun(t, f, t.TempDir())
	child, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parent, ParentRunID: run, Description: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	f.loop.release()
	_, err = f.subagents.Wait(context.Background(), parent, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.subagents.settings = &controlledSettings{SessionSettingsStore: f.settings, read: func() {
		once.Do(func() { close(entered) })
		<-release
	}}
	sendDone := make(chan struct{})
	var sendErr error
	go func() {
		defer close(sendDone)
		_, sendErr = f.subagents.Send(context.Background(), parent, run, child.TaskID, session.UserMessage{
			Blocks: []session.Block{{Kind: "text", Text: "second"}},
		})
	}()
	awaitSignal(t, entered)
	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		_ = f.subagents.Close()
	}()
	awaitSignal(t, f.subagents.ctx.Done())
	close(release)
	awaitSignal(t, sendDone)
	awaitSignal(t, closeDone)
	if !errors.Is(sendErr, ErrClosed) {
		t.Fatalf("send after close: %v", sendErr)
	}
	if _, active := f.runner.State(child.ChildSessionID); active {
		t.Fatal("close left child run active")
	}
}

func TestTaskRecordStaysRelationOnlyAfterCompletion(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	parent, run := createParentRun(t, f, t.TempDir())
	f.loop.release()
	child, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parent, ParentRunID: run, Description: "fast",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	response, err := f.subagents.Wait(context.Background(), parent, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err != nil || response.Tasks[0].Status != StatusCompleted || response.Tasks[0].ResultEntryID == "" {
		t.Fatalf("wait: %+v, %v", response, err)
	}
	record, err := f.subagents.store.loadTask(child.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != child.TaskID || record.ChildSessionID != child.ChildSessionID || record.Description != "fast" {
		t.Fatalf("wrong relation: %+v", record)
	}
}

func TestRecoveryProjectsHistoryFromChildSession(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	parent, run := createParentRun(t, f, t.TempDir())
	child, err := f.subagents.Spawn(context.Background(), SpawnInput{
		ParentSessionID: parent, ParentRunID: run, Description: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	f.loop.release()
	_, err = f.subagents.Wait(context.Background(), parent, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.subagents.Close(); err != nil {
		t.Fatal(err)
	}

	files, err := persist.NewFiles(filepath.Dir(f.subagents.store.dir))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := NewSubagents(f.sessions, f.settings, f.subagents.agents, f.subagents.models, f.runner, f.events, files)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	tasks, err := recovered.List(parent, child.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Status != StatusCompleted || len(tasks[0].Turns) != 1 || len(tasks[0].Results) != 1 {
		t.Fatalf("recovery lost child projection: %+v", tasks)
	}
	if !recovered.IsChildSession(child.ChildSessionID) {
		t.Fatal("recovery lost child relationship")
	}
}
