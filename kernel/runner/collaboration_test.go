package runner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"harness/kernel/events"
	"harness/kernel/loops"
	"harness/kernel/session"
)

func collaborationMessage() session.Message {
	return session.Message{Role: session.RoleCollaboration, MessageID: "task-1-turn-1", SourceSessionID: "child", SourceRunID: "child-run", Blocks: []session.Block{{Kind: "text", Text: "child answer"}}}
}

func TestCollaborationWaitsUntilToolResultsArePersisted(t *testing.T) {
	toolCallPersisted := make(chan struct{})
	persistToolResult := make(chan struct{})
	f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		assistant := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "tool-call", Tool: &session.ToolCall{ID: "wait-call", Name: "subagent_wait", Args: `{}`}}}}
		if err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: "assistant-call", Message: &assistant}); err != nil {
			return err
		}
		close(toolCallPersisted)
		<-persistToolResult

		toolResult := session.Message{Role: session.RoleTool, Blocks: []session.Block{{Kind: "tool-result", Result: &session.ToolResult{ID: "wait-call", Name: "subagent_wait", Content: `{"reason":"completed"}`}}}}
		if err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: "wait-result", Message: &toolResult}); err != nil {
			return err
		}
		messages, err := invocation.Checkpoint(ctx, loops.CheckpointContinue)
		if err != nil {
			return err
		}
		if len(messages) != 1 || messages[0].MessageID != collaborationMessage().MessageID {
			return fmt.Errorf("checkpoint messages = %#v", messages)
		}
		return nil
	}})
	defer f.runner.close()

	handle, err := f.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	<-toolCallPersisted

	accepted, err := f.runner.Receive("session-1", collaborationMessage())
	if accepted || err != nil {
		close(persistToolResult)
		handle.Wait()
		t.Fatalf("receive before checkpoint = %v, %v", accepted, err)
	}
	if entries := f.session.Entries(); len(entries) != 2 {
		t.Fatalf("collaboration entered unfinished tool batch: %#v", entries)
	}

	close(persistToolResult)
	if result := handle.Wait(); result.Err != nil {
		t.Fatal(result.Err)
	}
	entries := f.session.Entries()
	if len(entries) != 4 || entries[1].Message.Role != session.RoleAssistant || entries[2].Message.Role != session.RoleTool || entries[3].Message.Role != session.RoleCollaboration {
		t.Fatalf("ledger order = %#v", entries)
	}
	accepted, err = f.runner.Receive("session-1", collaborationMessage())
	if !accepted || err != nil {
		t.Fatalf("persisted collaboration was not acknowledged: %v, %v", accepted, err)
	}
}

func TestReceiveDeduplicatesAndRespectsFinalBoundary(t *testing.T) {
	started := make(chan loops.Invocation, 1)
	finish := make(chan struct{})
	f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		started <- invocation
		select {
		case <-finish:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}})
	defer f.runner.close()
	handle, err := f.runner.Start(context.Background(), "session-1", textInput("initial"))
	if err != nil {
		t.Fatal(err)
	}
	invocation := <-started
	signal := invocation.InputSignal()
	message := collaborationMessage()
	for i := 0; i < 3; i++ {
		accepted, receiveErr := f.runner.Receive("session-1", message)
		if accepted || receiveErr != nil {
			t.Fatalf("receive: %v, %v", accepted, receiveErr)
		}
	}
	select {
	case <-signal:
	default:
		t.Fatal("input signal not broadcast")
	}
	message.Blocks[0].Text = "mutated after receive"
	messages, err := invocation.Checkpoint(context.Background(), loops.CheckpointContinue)
	if err != nil || len(messages) != 1 || messages[0].Blocks[0].Text != "child answer" {
		t.Fatalf("checkpoint: %+v, %v", messages, err)
	}
	if len(f.session.Entries()) != 2 {
		t.Fatal("duplicate ledger input")
	}
	accepted, err := f.runner.Receive("session-1", collaborationMessage())
	if !accepted || err != nil {
		t.Fatalf("persisted collaboration was not acknowledged: %v, %v", accepted, err)
	}
	select {
	case <-invocation.InputSignal():
		t.Fatal("consumed signal still closed")
	default:
	}
	_, err = invocation.Checkpoint(context.Background(), loops.CheckpointFinal)
	if err != nil {
		t.Fatal(err)
	}
	late := collaborationMessage()
	late.MessageID = "late"
	accepted, err = f.runner.Receive("session-1", late)
	if accepted || err != nil {
		t.Fatalf("received after final checkpoint: %v, %v", accepted, err)
	}
	close(finish)
	result := handle.Wait()
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	accepted, err = f.runner.Receive("session-1", late)
	if accepted || err != nil || len(f.session.Entries()) != 2 {
		t.Fatal("idle receive changed ledger or started a run")
	}
}

func TestReceiveDuringStartupEntersHistoryOnce(t *testing.T) {
	seen := make(chan loops.Invocation, 2)
	f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		seen <- invocation
		messages, err := invocation.Checkpoint(ctx, loops.CheckpointFinal)
		if len(messages) != 0 {
			return errors.New("startup input repeated at checkpoint")
		}
		return err
	}})
	defer f.runner.close()
	_, err := events.Subscribe(f.events, func(_ context.Context, event RunEvent) error {
		if event.Kind != RunStarted {
			return nil
		}
		_, err := f.runner.Receive("session-1", collaborationMessage())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		err = f.runner.Run(context.Background(), "session-1", textInput("question"))
		if err != nil {
			t.Fatal(err)
		}
		invocation := <-seen
		count := 0
		for _, message := range invocation.History {
			if message.MessageID == collaborationMessage().MessageID {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("history contains %d collaboration messages", count)
		}
	}
}

func TestReceiveAppendFailureFailsRun(t *testing.T) {
	started := make(chan loops.Invocation, 1)
	checkpoint := make(chan struct{})
	f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		started <- invocation
		<-checkpoint
		_, err := invocation.Checkpoint(ctx, loops.CheckpointContinue)
		return err
	}})
	defer f.runner.close()
	handle, err := f.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	diskErr := errors.New("collaboration append failed")
	f.persistence.mu.Lock()
	f.persistence.addFail = diskErr
	f.persistence.mu.Unlock()
	accepted, err := f.runner.Receive("session-1", collaborationMessage())
	if accepted || err != nil {
		t.Fatalf("receive: %v, %v", accepted, err)
	}
	close(checkpoint)
	result := handle.Wait()
	if result.Status != RunFailed || !errors.Is(result.Err, diskErr) {
		t.Fatalf("result: %+v", result)
	}
	if len(f.session.Entries()) != 1 {
		t.Fatal("failed append reached ledger")
	}
}

func TestReceiveLatePublishFailureCannotReportSuccess(t *testing.T) {
	started := make(chan struct{})
	checkpoint := make(chan struct{})
	publishing := make(chan struct{})
	finishPublish := make(chan struct{})
	f := newRunnerFixture(t, &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		close(started)
		<-checkpoint
		_, err := invocation.Checkpoint(ctx, loops.CheckpointContinue)
		return err
	}})
	defer f.runner.close()
	publishErr := errors.New("collaboration publish failed")
	_, err := events.Subscribe(f.events, func(_ context.Context, event RunEvent) error {
		if event.Entry == nil || event.Entry.Message.Role != session.RoleCollaboration {
			return nil
		}
		close(publishing)
		<-finishPublish
		return publishErr
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := f.runner.Start(context.Background(), "session-1", textInput("question"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	accepted, err := f.runner.Receive("session-1", collaborationMessage())
	if accepted || err != nil {
		t.Fatalf("receive: %v, %v", accepted, err)
	}
	close(checkpoint)
	<-publishing
	select {
	case <-handle.Done():
		t.Fatal("completed before in-flight publication")
	default:
	}
	close(finishPublish)
	result := handle.Wait()
	if result.Status != RunFailed || !errors.Is(result.Err, publishErr) {
		t.Fatalf("result: %+v", result)
	}
	if len(f.session.Entries()) != 2 {
		t.Fatal("persisted input missing")
	}
}
