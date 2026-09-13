package runner

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"harness/kernel/events"
	"harness/kernel/loops"
	"harness/kernel/persist"
	"harness/kernel/session"
)

// 在读到旧记录或写入输入时暂停，稳定制造快照、收尾和插话交错。
type recoveryGatePersistence struct {
	persist.Persistence
	loadGate atomic.Bool
	addGate  atomic.Bool
	reached  chan struct{}
	resume   chan struct{}
}

func TestExpectedRunCannotEnterAfterFinalCheckpoint(t *testing.T) {
	closed, finish := make(chan struct{}), make(chan struct{})
	f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, in loops.Invocation) error {
		_, err := in.Checkpoint(ctx, loops.CheckpointFinal)
		close(closed)
		<-finish
		return err
	}})
	handle, err := f.runner.Start(t.Context(), "session-1", textInput("q"))
	if err != nil {
		t.Fatal(err)
	}
	<-closed
	err = f.runner.SteerRun("session-1", handle.RunID(), textInput("too late"))
	close(finish)
	handle.Wait()
	if !errors.Is(err, ErrRunChanged) || len(f.session.Entries()) != 1 {
		t.Fatalf("closed checkpoint accepted input: %v %+v", err, f.session.Entries())
	}
}

func (p *recoveryGatePersistence) LoadRunRecords(id string) ([]byte, error) {
	body, err := p.Persistence.LoadRunRecords(id)
	if p.loadGate.Swap(false) {
		close(p.reached)
		<-p.resume
	}
	return body, err
}

func (p *recoveryGatePersistence) Add(id string, node persist.Node) error {
	if p.addGate.Swap(false) {
		close(p.reached)
		<-p.resume
	}
	return p.Persistence.Add(id, node)
}

func TestSnapshotCannotOverwriteCompletedRecord(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	f := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error {
		close(started)
		<-finish
		return nil
	}})
	gate := &recoveryGatePersistence{Persistence: f.persistence, reached: make(chan struct{}), resume: make(chan struct{})}
	f.runner.persist = gate
	handle, err := f.runner.Start(context.Background(), "session-1", textInput("q"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	gate.loadGate.Store(true)
	snapshotDone := make(chan error, 1)
	go func() {
		_, err := f.runner.SessionView("session-1")
		snapshotDone <- err
	}()
	<-gate.reached
	close(finish)
	// 旧实现此时能写 success，随后被快照的旧 running 覆盖；新实现串行完成。
	select {
	case <-handle.Done():
	case <-time.After(20 * time.Millisecond):
	}
	close(gate.resume)
	if err := <-snapshotDone; err != nil {
		t.Fatal(err)
	}
	if result := handle.Wait(); result.Status != RunSucceeded {
		t.Fatal(result)
	}
	records, err := f.runner.loadRecords("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Status != RunSucceeded {
		t.Fatalf("successful run overwritten: %+v", records)
	}
}

func TestSnapshotCannotConsumeUnappliedRunEnded(t *testing.T) {
	f := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error { return nil }})
	runID, current, _, err := f.runner.openLive(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	defer f.runner.wg.Done()
	defer f.runner.release("session-1", current)
	entry, err := f.session.Append(messageFromInput(runID, textInput("q")))
	if err != nil {
		t.Fatal(err)
	}
	current.setAfterEntrySeq(entry.Seq)
	// 精确停在 liveEvent 已提交、Publish 尚未执行的位置。
	terminal := f.runner.liveEvent(current, RunEvent{SessionID: "session-1", RunID: runID, Kind: RunEnded, Status: RunSucceeded})
	view, err := f.runner.SessionView("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if view.UpdateSeq >= terminal.UpdateSeq && view.Runs[0].Status != RunSucceeded {
		t.Fatalf("snapshot filters completion but does not contain it: %+v", view)
	}
}

func TestAcceptedInputCannotMissFinalCheckpoint(t *testing.T) {
	for _, collaboration := range []bool{false, true} {
		name := "steer"
		if collaboration {
			name = "collaboration"
		}
		t.Run(name, func(t *testing.T) {
			started, checkpoint := make(chan struct{}), make(chan struct{})
			consumed := make(chan []session.Message, 1)
			f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, in loops.Invocation) error {
				close(started)
				<-checkpoint
				messages, err := in.Checkpoint(ctx, loops.CheckpointFinal)
				consumed <- messages
				return err
			}})
			handle, err := f.runner.Start(context.Background(), "session-1", textInput("q"))
			if err != nil {
				t.Fatal(err)
			}
			<-started
			inputDone := make(chan error, 1)
			go func() {
				if collaboration {
					_, err := f.runner.Receive("session-1", collaborationMessage())
					inputDone <- err
					return
				}
				inputDone <- f.runner.Steer("session-1", textInput("new question"))
			}()
			waitPendingInputCount(t, f.runner, "session-1", 1)
			close(checkpoint)
			if err := <-inputDone; err != nil {
				t.Fatal(err)
			}
			if result := handle.Wait(); result.Err != nil {
				t.Fatal(result.Err)
			}
			messages := <-consumed
			if len(messages) != 1 {
				t.Fatalf("accepted input not consumed: %+v", messages)
			}
		})
	}
}

func TestNewOutputFollowsConsumedSteer(t *testing.T) {
	ready, resume := make(chan struct{}), make(chan struct{})
	f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, in loops.Invocation) error {
		close(ready)
		<-resume
		if _, err := in.Checkpoint(ctx, loops.CheckpointContinue); err != nil {
			return err
		}
		if err := in.Emit(ctx, loops.Event{Kind: loops.EventMessageStarted, EntryID: "after-steer"}); err != nil {
			return err
		}
		message := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "reply"}}}
		return in.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: "after-steer", Message: &message})
	}})
	handle, err := f.runner.Start(context.Background(), "session-1", textInput("q"))
	if err != nil {
		t.Fatal(err)
	}
	<-ready
	steerDone := make(chan error, 1)
	go func() { steerDone <- f.runner.Steer("session-1", textInput("new question")) }()
	waitPendingInputCount(t, f.runner, "session-1", 1)
	close(resume)
	if err := <-steerDone; err != nil {
		t.Fatal(err)
	}
	handle.Wait()
	entries := f.session.Entries()
	if entries[2].Message.AfterSeq != entries[1].Seq {
		t.Fatalf("new output anchor=%d, consumed input=%d", entries[2].Message.AfterSeq, entries[1].Seq)
	}
}

func TestRunWaitsForSteerPublicationBeforeReleasingClock(t *testing.T) {
	started, publishing := make(chan struct{}), make(chan struct{})
	checkpoint, resume := make(chan struct{}), make(chan struct{})
	f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		close(started)
		<-checkpoint
		_, err := invocation.Checkpoint(ctx, loops.CheckpointContinue)
		return err
	}})
	publishErr := errors.New("steer notification failed")
	_, err := events.Subscribe(f.events, func(_ context.Context, event RunEvent) error {
		if event.Entry != nil && event.Entry.Seq == 2 {
			close(publishing)
			<-resume
			return publishErr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := f.runner.Start(context.Background(), "session-1", textInput("q"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	sent := make(chan error, 1)
	go func() { sent <- f.runner.Steer("session-1", textInput("steer")) }()
	waitPendingInputCount(t, f.runner, "session-1", 1)
	close(checkpoint)
	<-publishing
	select {
	case <-handle.Done():
		t.Error("run released while an input notification was still publishing")
	case <-time.After(20 * time.Millisecond):
	}
	close(resume)
	if err := <-sent; !errors.Is(err, publishErr) {
		t.Fatalf("caller lost publication error: %v", err)
	}
	if result := handle.Wait(); result.Status != RunFailed || !errors.Is(result.Err, publishErr) {
		t.Fatalf("steer publication failure must fail the run: %+v", result)
	}
}
