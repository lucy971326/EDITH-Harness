package runner

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"harness/kernel/events"
	"harness/kernel/loops"
	"harness/kernel/session"
)

func TestDraftSnapshotAndPersistedEntryShareEntryID(t *testing.T) {
	entryID := "msg-1"
	started := make(chan struct{})
	continueRun := make(chan struct{})
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessageStarted, EntryID: entryID})
		if err != nil {
			return err
		}
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, EntryID: entryID, BlockSeq: 1, Text: "你好"})
		if err != nil {
			return err
		}
		close(started)
		<-continueRun
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, EntryID: entryID, BlockSeq: 1, Text: "世界"})
		if err != nil {
			return err
		}
		message := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "你好世界"}}}
		return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: entryID, Message: &message})
	}}
	fixture := newRunnerFixture(t, loop)
	var firstDelta RunEvent
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		if event.Kind == TextDelta && firstDelta.EntryID == "" {
			firstDelta = event
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- fixture.runner.Run(context.Background(), "session-1", textInput("问"))
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("loop did not start")
	}
	view, err := fixture.runner.SessionView("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if firstDelta.EntryID != entryID {
		t.Fatalf("first delta id = %q", firstDelta.EntryID)
	}
	if len(view.Runs) != 1 || len(view.Runs[0].Drafts) != 1 {
		t.Fatalf("draft snapshot = %#v", view.Runs)
	}
	draft := view.Runs[0].Drafts[0]
	if draft.EntryID != entryID || len(draft.Blocks) != 1 || draft.Blocks[0].Text != "你好" {
		t.Fatalf("draft = %#v", draft)
	}
	close(continueRun)
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not finish")
	}
	view, err = fixture.runner.SessionView("session-1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range view.Entries {
		if entry.ID != entryID {
			continue
		}
		found = true
		if entry.Message.Blocks[0].Text != "你好世界" || entry.Message.Incomplete {
			t.Fatalf("final entry = %#v", entry)
		}
	}
	if !found {
		t.Fatalf("missing persisted entry %s: %#v", entryID, view.Entries)
	}
	if len(view.Runs) != 1 || view.Runs[0].Status != RunSucceeded || len(view.Runs[0].Drafts) != 0 {
		t.Fatalf("completed run = %#v", view.Runs)
	}
}

func TestTwoModelOutputsGetDifferentEntryIDs(t *testing.T) {
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		first := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{
			{Kind: "tool-call", Tool: &session.ToolCall{ID: "call-1", Name: "read", Args: `{}`}},
		}}
		err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: "assist-1", Message: &first})
		if err != nil {
			return err
		}
		result := session.Message{Role: session.RoleTool, Blocks: []session.Block{{Kind: "tool-result", Result: &session.ToolResult{ID: "call-1", Name: "read", Content: "ok"}}}}
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: "result-1", Message: &result})
		if err != nil {
			return err
		}
		second := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "done"}}}
		return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: "assist-2", Message: &second})
	}}
	fixture := newRunnerFixture(t, loop)
	err := fixture.runner.Run(context.Background(), "session-1", textInput("go"))
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0)
	for _, entry := range fixture.session.Entries() {
		if entry.Message.Role == session.RoleAssistant {
			ids = append(ids, entry.ID)
		}
	}
	if len(ids) != 2 || ids[0] != "assist-1" || ids[1] != "assist-2" {
		t.Fatalf("assistant ids = %#v", ids)
	}
}

func TestSnapshotDuringPersistHasNoDuplicateOrGap(t *testing.T) {
	entryID := "stream-1"
	var snapshots []SessionView
	var mu sync.Mutex
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessageStarted, EntryID: entryID})
		if err != nil {
			return err
		}
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, EntryID: entryID, BlockSeq: 1, Text: "abc"})
		if err != nil {
			return err
		}
		message := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "abc"}}}
		return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: entryID, Message: &message})
	}}
	fixture := newRunnerFixture(t, loop)
	stop := make(chan struct{})
	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		first := true
		for {
			select {
			case <-stop:
				return
			default:
			}
			view, err := fixture.runner.SessionView("session-1")
			if err != nil {
				continue
			}
			mu.Lock()
			snapshots = append(snapshots, view)
			mu.Unlock()
			if first {
				close(started)
				first = false
			}
		}
	}()
	<-started
	err := fixture.runner.Run(context.Background(), "session-1", textInput("q"))
	if err != nil {
		t.Fatal(err)
	}
	close(stop)
	wg.Wait()
	if len(snapshots) == 0 {
		t.Fatal("no snapshots")
	}
	for _, view := range snapshots {
		hasEntry := false
		hasDraft := false
		for _, entry := range view.Entries {
			if entry.ID == entryID {
				hasEntry = true
			}
		}
		for _, run := range view.Runs {
			for _, draft := range run.Drafts {
				if draft.EntryID == entryID {
					hasDraft = true
				}
			}
		}
		if hasEntry && hasDraft {
			t.Fatalf("duplicate draft and entry: %#v", view)
		}
	}
}

func TestLateDeltaAfterPersistDoesNotRecreateDraft(t *testing.T) {
	entryID := "done-1"
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		message := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "ok"}}}
		err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: entryID, Message: &message})
		if err != nil {
			return err
		}
		return invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, EntryID: entryID, BlockSeq: 1, Text: "late"})
	}}
	fixture := newRunnerFixture(t, loop)
	err := fixture.runner.Run(context.Background(), "session-1", textInput("q"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := fixture.runner.SessionView("session-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range view.Runs {
		if len(run.Drafts) != 0 {
			t.Fatalf("late delta recreated draft: %#v", run.Drafts)
		}
	}
	text := ""
	for _, entry := range view.Entries {
		if entry.ID == entryID {
			text = entry.Message.Blocks[0].Text
		}
	}
	if text != "ok" {
		t.Fatalf("persisted text = %q", text)
	}
}

func TestStopPersistsIncompleteWithOriginalEntryID(t *testing.T) {
	entryID := "partial-1"
	started := make(chan struct{})
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessageStarted, EntryID: entryID})
		if err != nil {
			return err
		}
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, EntryID: entryID, BlockSeq: 1, Text: "半截"})
		if err != nil {
			return err
		}
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	fixture := newRunnerFixture(t, loop)
	handle, err := fixture.runner.Start(context.Background(), "session-1", textInput("q"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("loop did not start")
	}
	err = fixture.runner.Stop("session-1")
	if err != nil {
		t.Fatal(err)
	}
	result := handle.Wait()
	if result.Status != RunCancelled {
		t.Fatalf("result = %#v", result)
	}
	view, err := fixture.runner.SessionView("session-1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range view.Entries {
		if entry.ID != entryID {
			continue
		}
		found = true
		if !entry.Message.Incomplete || entry.Message.Blocks[0].Text != "半截" {
			t.Fatalf("incomplete entry = %#v", entry)
		}
	}
	if !found {
		t.Fatalf("missing incomplete entry: %#v", view.Entries)
	}
	if view.Runs[len(view.Runs)-1].Status != RunCancelled {
		t.Fatalf("runs = %#v", view.Runs)
	}
	history := fixture.session.History()
	last := history[len(history)-1]
	if last.Blocks[len(last.Blocks)-1].Text != "（未完成）" {
		t.Fatalf("history = %#v", history)
	}
}

func TestRunningRecordBecomesInterruptedWithoutLiveRun(t *testing.T) {
	fixture := newRunnerFixture(t, &runnerTestLoop{run: func(context.Context, loops.Invocation) error { return nil }})
	err := fixture.runner.upsertRecord("session-1", runRecord{RunID: "old-run", Status: RunRunning, AfterEntrySeq: 1})
	if err != nil {
		t.Fatal(err)
	}
	view, err := fixture.runner.SessionView("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Runs) != 1 || view.Runs[0].Status != RunInterrupted || view.Runs[0].RunID != "old-run" {
		t.Fatalf("interrupted = %#v", view.Runs)
	}
	again, err := fixture.runner.loadRecords("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Status != RunInterrupted {
		t.Fatalf("disk = %#v", again)
	}
}

func TestSteerDuringDraftKeepsAfterSeq(t *testing.T) {
	entryID := "gen-1"
	started := make(chan struct{})
	continueRun := make(chan struct{})
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessageStarted, EntryID: entryID})
		if err != nil {
			return err
		}
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, EntryID: entryID, BlockSeq: 1, Text: "正在"})
		if err != nil {
			return err
		}
		close(started)
		<-continueRun
		message := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "正在写"}}}
		if err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: entryID, Message: &message}); err != nil {
			return err
		}
		_, err = invocation.Checkpoint(ctx, loops.CheckpointFinal)
		return err
	}}
	fixture := newRunnerFixture(t, loop)
	done := make(chan error, 1)
	go func() {
		done <- fixture.runner.Run(context.Background(), "session-1", textInput("问"))
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("loop did not start")
	}
	steerDone := make(chan error, 1)
	go func() { steerDone <- fixture.runner.Steer("session-1", textInput("插话")) }()
	waitPendingInputCount(t, fixture.runner, "session-1", 1)
	view, err := fixture.runner.SessionView("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Runs[0].Drafts) != 1 || view.Runs[0].Drafts[0].AfterEntrySeq != 1 {
		t.Fatalf("draft after steer = %#v", view.Runs[0].Drafts)
	}
	close(continueRun)
	if err = <-steerDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not finish")
	}
	var assistant session.Entry
	for _, entry := range fixture.session.Entries() {
		if entry.ID == entryID {
			assistant = entry
		}
	}
	if assistant.ID == "" || assistant.Message.AfterSeq != 1 {
		t.Fatalf("persisted afterSeq = %#v", assistant)
	}
	if assistant.Seq != 2 {
		t.Fatalf("assistant must finish before Steer, seq=%d", assistant.Seq)
	}
}

func TestBufferedEventsAfterSnapshotCanBeFilteredByUpdateSeq(t *testing.T) {
	entryID := "seq-1"
	var eventsSeen []RunEvent
	var snapshotSeq atomic.Uint64
	var fixture runnerFixture
	loop := &runnerTestLoop{run: func(ctx context.Context, invocation loops.Invocation) error {
		err := invocation.Emit(ctx, loops.Event{Kind: loops.EventMessageStarted, EntryID: entryID})
		if err != nil {
			return err
		}
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, EntryID: entryID, BlockSeq: 1, Text: "a"})
		if err != nil {
			return err
		}
		view, err := fixture.runner.SessionView("session-1")
		if err != nil {
			return err
		}
		snapshotSeq.Store(view.UpdateSeq)
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, EntryID: entryID, BlockSeq: 1, Text: "b"})
		if err != nil {
			return err
		}
		message := session.Message{Role: session.RoleAssistant, Blocks: []session.Block{{Kind: "text", Text: "ab"}}}
		return invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, EntryID: entryID, Message: &message})
	}}
	fixture = newRunnerFixture(t, loop)
	_, err := events.Subscribe(fixture.events, func(_ context.Context, event RunEvent) error {
		eventsSeen = append(eventsSeen, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.runner.Run(context.Background(), "session-1", textInput("q"))
	if err != nil {
		t.Fatal(err)
	}
	boundary := snapshotSeq.Load()
	if boundary == 0 {
		t.Fatal("snapshot seq was 0")
	}
	var replayed strings.Builder
	applied := 0
	for _, event := range eventsSeen {
		if event.UpdateSeq <= boundary {
			continue
		}
		applied++
		if event.Kind == TextDelta {
			replayed.WriteString(event.Text)
		}
	}
	if applied == 0 {
		t.Fatal("no events after snapshot boundary")
	}
	if replayed.String() != "b" {
		t.Fatalf("replayed deltas = %q, want b only", replayed.String())
	}
}
