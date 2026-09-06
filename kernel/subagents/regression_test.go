package subagents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 活对象。确认 Wait 已捕获目标轮次并进入 select，不靠调度延时猜测。
type observedWaitContext struct {
	context.Context
	once    sync.Once
	entered chan struct{}
}

func (c *observedWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

// 活对象。通过已有设置存储契约复现启动阻塞和写入失败。
type controlledSettings struct {
	settings.SessionSettingsStore
	read  func()
	write func(string) error
}

func (s *controlledSettings) For(id string) (settings.SessionSettings, error) {
	if s.read != nil {
		s.read()
	}
	return s.SessionSettingsStore.For(id)
}

func (s *controlledSettings) Put(id string, setup settings.SessionSettings) error {
	if s.write != nil {
		err := s.write(id)
		if err != nil {
			return err
		}
	}
	return s.SessionSettingsStore.Put(id, setup)
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
	f.subagents.settings = &controlledSettings{SessionSettingsStore: f.settings, read: func() {
		close(entered)
		<-release
	}}
	sendDone := make(chan struct{})
	var sendErr error
	go func() {
		defer close(sendDone)
		_, sendErr = f.subagents.Send(context.Background(), parent, child.TaskID, session.UserMessage{
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
	records, err := f.subagents.store.listTasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range records {
		if task.Status == StatusRunning || task.Status == StatusPending {
			t.Fatalf("close left unfinished task: %+v", task)
		}
	}
}

func TestSpawnSaveFailuresRemainObservable(t *testing.T) {
	for _, stage := range []string{"settings", "running"} {
		t.Run(stage, func(t *testing.T) {
			f := newSubagentsFixture(t)
			defer f.host.Close()
			parent, run := createParentRun(t, f, t.TempDir())
			f.subagents.settings = &controlledSettings{
				SessionSettingsStore: f.settings,
				write: func(childID string) error {
					// 关系已落盘，用真实文件系统障碍阻断之后的状态保存。
					f.subagents.mu.RLock()
					id := f.subagents.childSessions[childID]
					f.subagents.mu.RUnlock()
					err := os.Mkdir(filepath.Join(f.subagents.store.dir, id+".json.tmp"), 0o700)
					if err != nil {
						return err
					}
					if stage == "settings" {
						return errors.New("settings write failed")
					}
					return nil
				},
			}
			_, err := f.subagents.Spawn(context.Background(), SpawnInput{
				ParentSessionID: parent, ParentRunID: run, Description: stage,
			})
			if !errors.Is(err, ErrPersistFailed) {
				t.Fatalf("spawn must report persistence failure: %v", err)
			}
			tasks, listErr := f.subagents.List(parent, "")
			if listErr == nil || len(tasks) != 1 {
				t.Fatalf("list lost failure: %v, %+v", listErr, tasks)
			}
			_, err = f.subagents.Wait(context.Background(), parent, WaitInput{TaskIDs: []string{tasks[0].ID}, Timeout: time.Second})
			if !errors.Is(err, ErrPersistFailed) {
				t.Fatalf("wait lost failure: %v", err)
			}
			first := f.subagents.Close()
			second := f.subagents.Close()
			if first == nil || first != second {
				t.Fatalf("close lost failure: %v / %v", first, second)
			}
		})
	}
}

func TestFastCompletionPersistsTerminalState(t *testing.T) {
	f := newSubagentsFixture(t)
	defer f.host.Close()
	parent, run := createParentRun(t, f, t.TempDir())
	for i := 0; i < 30; i++ {
		f.loop.release()
		child, err := f.subagents.Spawn(context.Background(), SpawnInput{
			ParentSessionID: parent, ParentRunID: run, Description: "fast",
		})
		if err != nil {
			t.Fatal(err)
		}
		f.loop.waitStarted(t)
		res, err := f.subagents.Wait(context.Background(), parent, WaitInput{TaskIDs: []string{child.TaskID}, Timeout: time.Second})
		if err != nil || res.Tasks[0].Status != StatusCompleted {
			t.Fatalf("wait: %+v, %v", res, err)
		}
		task, err := f.subagents.store.loadTask(child.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status != StatusCompleted || len(task.Notifications) != 1 || task.ResultEntryID == "" {
			t.Fatalf("disk overwritten by stale running state: %+v", task)
		}
	}
}

func TestRecoveryKeepsHistoryWithoutResuming(t *testing.T) {
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
	_, err = f.subagents.Send(context.Background(), parent, child.TaskID, session.UserMessage{
		Blocks: []session.Block{{Kind: "text", Text: "second"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.waitStarted(t)
	snapshot, err := f.subagents.store.loadTask(child.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	store, err := newTaskStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = store.saveTask(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := NewSubagents(f.sessions, f.settings, f.subagents.agents, f.subagents.models, f.runner, f.events, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	tasks, err := recovered.List(parent, child.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	task := tasks[0]
	if task.Status != StatusInterrupted || task.Turns[1].Status != StatusInterrupted ||
		task.Turns[0].Status != StatusCompleted || task.Turns[0].ResultEntryID == "" ||
		len(task.Notifications) != 2 || task.Notifications[0].NotificationID != snapshot.Notifications[0].NotificationID || task.Notifications[1].Status != StatusInterrupted {
		t.Fatalf("recovery lost history: %+v", task)
	}
	if !recovered.IsChildSession(child.ChildSessionID) || recovered.coords[child.TaskID].activeHandle != nil {
		t.Fatal("recovery must restore relationship, not execution")
	}
	persisted, err := store.loadTask(child.TaskID)
	if err != nil || persisted.Status != StatusInterrupted {
		t.Fatalf("interruption not saved: %+v, %v", persisted, err)
	}
}
