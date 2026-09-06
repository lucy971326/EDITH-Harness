package runner

import (
	"context"
	"errors"
	"testing"

	"harness/kernel/events"
	"harness/kernel/loops"
	"harness/kernel/session/settings"
)

// 活对象。阻塞准备阶段的真实设置契约包装器。
type preparingSettings struct {
	settings.SessionSettingsStore
	entered chan struct{}
	release chan struct{}
}

func TestStopFromRunStartedPreventsLoop(t *testing.T) {
	started := make(chan struct{}, 1)
	f := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error { started <- struct{}{}; return nil }})
	defer f.runner.close()
	_, err := events.Subscribe(f.events, func(_ context.Context, event RunEvent) error {
		if event.Kind == RunStarted {
			f.runner.StopRun(event.SessionID, event.RunID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := f.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunCancelled {
		t.Fatalf("result: %+v", result)
	}
	select {
	case <-started:
		t.Fatal("stopped startup entered Loop")
	default:
	}
}

func TestStopRunDoesNotCancelDifferentRun(t *testing.T) {
	started := make(chan struct{})
	f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, _ loops.Invocation) error { close(started); <-ctx.Done(); return ctx.Err() }})
	defer f.runner.close()
	handle, err := f.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if f.runner.StopRun("session-1", "old-run") {
		t.Fatal("old identity cancelled new run")
	}
	select {
	case <-handle.Done():
		t.Fatal("wrong run stopped")
	default:
	}
	if !f.runner.StopRun("session-1", handle.RunID()) {
		t.Fatal("matching run not cancelled")
	}
	if handle.Wait().Status != RunCancelled {
		t.Fatal("run did not cancel")
	}
}

func (s *preparingSettings) For(id string) (settings.SessionSettings, error) {
	close(s.entered)
	<-s.release
	return s.SessionSettingsStore.For(id)
}

func TestStopDuringPreparationPreventsLoop(t *testing.T) {
	started := make(chan struct{}, 1)
	f := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error { started <- struct{}{}; return nil }})
	defer f.runner.close()
	barrier := &preparingSettings{SessionSettingsStore: f.settings, entered: make(chan struct{}), release: make(chan struct{})}
	f.runner.settings = barrier
	done := make(chan error, 1)
	go func() {
		handle, err := f.runner.Start(context.Background(), "session-1", textInput("question"))
		if err == nil {
			err = handle.Wait().Err
		}
		done <- err
	}()
	<-barrier.entered
	err := f.runner.Stop("session-1")
	if err != nil {
		t.Error("preparing Run was not cancellable:", err)
	}
	close(barrier.release)
	err = <-done
	if !errors.Is(err, context.Canceled) {
		t.Errorf("preparation continued after stop: %v", err)
	}
	select {
	case <-started:
		t.Error("Loop executed after stop")
	default:
	}
}
