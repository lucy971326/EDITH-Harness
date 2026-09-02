package runner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"harness/kernel/loops"
	"harness/kernel/session"
)

func TestSteerWaitsForCheckpointAndCopiesInput(t *testing.T) {
	ready := make(chan struct{})
	continueRun := make(chan struct{})
	checkpointMessages := make(chan []session.Message, 1)
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		close(ready)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-continueRun:
		}
		messages, err := invocation.Checkpoint(ctx, loops.CheckpointFinal)
		if err != nil {
			return err
		}
		checkpointMessages <- messages
		_, err = invocation.Checkpoint(ctx, loops.CheckpointFinal)
		return err
	}}
	fixture := newRunnerFixture(t, loop)
	runDone := make(chan error, 1)
	go func() {
		runDone <- fixture.runner.Run(context.Background(), "session-1", textInput("initial"))
	}()
	<-ready

	steer := textInput("steer")
	err := fixture.runner.Steer("session-1", steer)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.session.History()) != 1 {
		t.Fatal("steer was appended before checkpoint")
	}
	steer.Blocks[0].Text = "mutated"
	err = fixture.runner.Steer("session-1", textInput("second steer"))
	if err != nil {
		t.Fatal(err)
	}
	close(continueRun)

	select {
	case err = <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not finish")
	}
	messages := <-checkpointMessages
	if len(messages) != 2 || messages[0].Blocks[0].Text != "steer" || messages[1].Blocks[0].Text != "second steer" {
		t.Fatalf("checkpoint messages = %#v", messages)
	}
	history := fixture.session.History()
	if len(history) != 3 || history[1].Blocks[0].Text != "steer" || history[2].Blocks[0].Text != "second steer" {
		t.Fatalf("history = %#v", history)
	}
}

func TestSteerCannotEnterAfterEmptyFinalCheckpoint(t *testing.T) {
	finalCheckpoint := make(chan struct{})
	finishRun := make(chan struct{})
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		messages, err := invocation.Checkpoint(ctx, loops.CheckpointFinal)
		if err != nil {
			return err
		}
		if len(messages) != 0 {
			return errors.New("unexpected steer")
		}
		close(finalCheckpoint)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-finishRun:
			return nil
		}
	}}
	fixture := newRunnerFixture(t, loop)
	runDone := make(chan error, 1)
	go func() {
		runDone <- fixture.runner.Run(context.Background(), "session-1", textInput("initial"))
	}()
	<-finalCheckpoint

	err := fixture.runner.Steer("session-1", textInput("too late"))
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("Steer error = %v", err)
	}
	close(finishRun)
	select {
	case err = <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not finish")
	}
	if len(fixture.session.History()) != 1 {
		t.Fatal("late steer reached the ledger")
	}
}

func TestConcurrentRunIsRejectedAndStopCancelsCurrentRun(t *testing.T) {
	ready := make(chan struct{})
	loop := &runnerTestLoop{run: func(ctx context.Context, _ loops.Invocation) error {
		close(ready)
		<-ctx.Done()
		return ctx.Err()
	}}
	fixture := newRunnerFixture(t, loop)
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- fixture.runner.Run(context.Background(), "session-1", textInput("first"))
	}()
	<-ready

	err := fixture.runner.Run(context.Background(), "session-1", textInput("second"))
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second run error = %v", err)
	}
	err = fixture.runner.Stop("session-1")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel run")
	}
}

func TestSteerAndStopRejectIdleSession(t *testing.T) {
	loop := &runnerTestLoop{run: func(context.Context, loops.Invocation) error { return nil }}
	fixture := newRunnerFixture(t, loop)
	if err := fixture.runner.Steer("session-1", textInput("late")); err == nil {
		t.Fatal("expected idle Steer error")
	}
	if err := fixture.runner.Stop("session-1"); err == nil {
		t.Fatal("expected idle Stop error")
	}
}

func TestFollowUpStartsNextRunAfterCurrentRunEnds(t *testing.T) {
	ready := make(chan struct{})
	release := make(chan struct{})
	var invocations []loops.Invocation
	var mu sync.Mutex
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		mu.Lock()
		invocations = append(invocations, invocation)
		count := len(invocations)
		mu.Unlock()
		if count == 1 {
			close(ready)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release:
			}
		}
		return nil
	}}
	fixture := newRunnerFixture(t, loop)
	done := make(chan error, 1)
	go func() { done <- fixture.runner.Run(context.Background(), "session-1", textInput("first")) }()
	<-ready
	if err := fixture.runner.FollowUp("session-1", textInput("second")); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("follow up did not finish")
	}
	if len(invocations) != 2 {
		t.Fatalf("invocations = %d", len(invocations))
	}
	if got := invocations[1].History; len(got) != 2 || got[1].Blocks[0].Text != "second" {
		t.Fatalf("second history = %#v", got)
	}
}

func TestCloseCancelsAndWaitsForManagedRun(t *testing.T) {
	ready := make(chan struct{})
	loop := &runnerTestLoop{run: func(ctx context.Context, _ loops.Invocation) error {
		close(ready)
		<-ctx.Done()
		return ctx.Err()
	}}
	fixture := newRunnerFixture(t, loop)
	done := make(chan error, 1)
	go func() { done <- fixture.runner.Run(context.Background(), "session-1", textInput("first")) }()
	<-ready
	fixture.runner.close()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not wait for run")
	}
}
