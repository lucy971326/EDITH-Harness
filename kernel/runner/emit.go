package runner

import (
	"context"
	"fmt"

	"harness/kernel/loops"
	"harness/kernel/session"
)

func (r *Runner) emit(ctx context.Context, sessionID, runID string, sess *session.Session, current *liveRun, event loops.Event) error {
	switch event.Kind {
	case loops.EventMessageStarted:
		return r.startDraft(ctx, sessionID, runID, current, event.EntryID)
	case loops.EventTextDelta, loops.EventReasoningDelta:
		return r.applyDelta(ctx, sessionID, runID, current, event)
	case loops.EventMessage:
		return r.persistLoopMessage(ctx, sessionID, runID, sess, current, event)
	case loops.EventToolStarted, loops.EventToolFinished:
		runEvent, err := mapEvent(sessionID, runID, event)
		if err != nil {
			return err
		}
		if event.Tool != nil {
			position := current.toolCall(event.Tool.ID)
			if position.entryID != "" {
				runEvent.EntryID = position.entryID
				runEvent.BlockSeq = position.blockSeq
			}
		}
		runEvent.AfterEntrySeq = current.afterSeq()
		return r.publish(ctx, r.liveEvent(current, runEvent))
	case loops.EventUsage:
		runEvent, err := mapEvent(sessionID, runID, event)
		if err != nil {
			return err
		}
		runEvent.AfterEntrySeq = current.afterSeq()
		return r.publishUsage(ctx, sessionID, current, runEvent)
	default:
		return fmt.Errorf("runner: unsupported loop event %q", event.Kind)
	}
}

func (r *Runner) publishUsage(ctx context.Context, sessionID string, current *liveRun, event RunEvent) error {
	event = r.liveEvent(current, event)
	current.mu.Lock()
	record := runRecord{
		RunID:         current.runID,
		Status:        RunRunning,
		AfterEntrySeq: current.afterEntrySeq,
		Usage:         cloneUsage(current.usage),
	}
	current.mu.Unlock()
	err := r.upsertRecord(sessionID, record)
	if err != nil {
		return err
	}
	return r.publish(ctx, event)
}

func (r *Runner) startDraft(ctx context.Context, sessionID, runID string, current *liveRun, entryID string) error {
	if entryID == "" {
		return fmt.Errorf("runner: message start needs an entry id")
	}
	current.mu.Lock()
	if _, exists := current.persisted[entryID]; exists {
		current.mu.Unlock()
		return nil
	}
	if _, exists := current.drafts[entryID]; !exists {
		current.drafts[entryID] = &runDraft{entryID: entryID, afterEntrySeq: current.outputAfterSeq}
	}
	current.updateSeq++
	seq := current.updateSeq
	after := current.drafts[entryID].afterEntrySeq
	current.mu.Unlock()
	return r.publish(ctx, RunEvent{
		SessionID:     sessionID,
		RunID:         runID,
		Kind:          MessageStarted,
		EntryID:       entryID,
		AfterEntrySeq: after,
		UpdateSeq:     seq,
		SeqEpoch:      r.epoch,
	})
}

func (r *Runner) applyDelta(ctx context.Context, sessionID, runID string, current *liveRun, event loops.Event) error {
	if event.EntryID == "" {
		return fmt.Errorf("runner: delta needs an entry id")
	}
	kind := "text"
	runKind := TextDelta
	if event.Kind == loops.EventReasoningDelta {
		kind = "reasoning"
		runKind = ReasoningDelta
	}
	current.mu.Lock()
	if _, exists := current.persisted[event.EntryID]; exists {
		current.mu.Unlock()
		return nil
	}
	draft := current.drafts[event.EntryID]
	if draft != nil && event.Text != "" {
		draft.blocks = applyDraftText(draft.blocks, kind, event.Text, event.BlockSeq)
	}
	current.updateSeq++
	seq := current.updateSeq
	after := current.afterEntrySeq
	if draft != nil {
		after = draft.afterEntrySeq
	}
	current.mu.Unlock()
	return r.publish(ctx, RunEvent{
		SessionID:     sessionID,
		RunID:         runID,
		Kind:          runKind,
		EntryID:       event.EntryID,
		AfterEntrySeq: after,
		BlockSeq:      event.BlockSeq,
		Text:          event.Text,
		UpdateSeq:     seq,
		SeqEpoch:      r.epoch,
	})
}

func (r *Runner) persistLoopMessage(ctx context.Context, sessionID, runID string, sess *session.Session, current *liveRun, event loops.Event) error {
	if event.Message == nil {
		return fmt.Errorf("runner: message event has nil message")
	}
	message := *event.Message
	message.RunID = runID
	entryID := event.EntryID
	if entryID == "" {
		id, err := session.NewEntryID()
		if err != nil {
			return err
		}
		entryID = id
	}

	current.mu.Lock()
	_, already := current.persisted[entryID]
	current.mu.Unlock()
	if already {
		return nil
	}

	if message.Role == session.RoleAssistant && message.AfterSeq == 0 {
		current.mu.Lock()
		if draft := current.drafts[entryID]; draft != nil {
			message.AfterSeq = draft.afterEntrySeq
		} else {
			message.AfterSeq = current.outputAfterSeq
		}
		current.mu.Unlock()
	}

	current.handoff.Lock()
	entry, err := sess.AppendID(entryID, message)
	current.mu.Lock()
	if err == nil {
		delete(current.drafts, entryID)
		current.persisted[entryID] = struct{}{}
		if message.Role == session.RoleAssistant {
			current.registerToolCallsLocked(entryID, message)
		}
		current.updateSeq++
	}
	seq := current.updateSeq
	after := current.afterEntrySeq
	if message.Role == session.RoleAssistant {
		after = message.AfterSeq
	}
	current.mu.Unlock()
	current.handoff.Unlock()
	if err != nil {
		return err
	}

	out := RunEvent{
		SessionID:     sessionID,
		RunID:         runID,
		Kind:          Message,
		EntryID:       entry.ID,
		AfterEntrySeq: after,
		Entry:         &entry,
		UpdateSeq:     seq,
		SeqEpoch:      r.epoch,
	}
	if message.Role == session.RoleTool && event.Tool == nil && len(message.Blocks) == 1 && message.Blocks[0].Result != nil {
		position := current.toolCall(message.Blocks[0].Result.ID)
		out.BlockSeq = position.blockSeq
	} else if event.BlockSeq != 0 && message.Role != session.RoleTool {
		out.BlockSeq = event.BlockSeq
	}
	return r.publish(ctx, out)
}

func (r *Runner) persistOpenDrafts(sess *session.Session, current *liveRun, sessionID, runID string) error {
	type persisted struct {
		entry session.Entry
		after uint64
		seq   uint64
	}

	current.handoff.Lock()
	current.mu.Lock()
	ids := make([]string, 0, len(current.drafts))
	for id := range current.drafts {
		ids = append(ids, id)
	}
	current.mu.Unlock()

	var first error
	out := make([]persisted, 0, len(ids))
	for _, id := range ids {
		current.mu.Lock()
		draft := current.drafts[id]
		if draft == nil {
			current.mu.Unlock()
			continue
		}
		message := incompleteFromDraft(*draft)
		after := draft.afterEntrySeq
		current.mu.Unlock()
		if message.Role == "" {
			current.mu.Lock()
			delete(current.drafts, id)
			current.mu.Unlock()
			continue
		}
		message.RunID = runID
		message.AfterSeq = after
		entry, err := sess.AppendID(id, message)
		current.mu.Lock()
		if err != nil {
			if first == nil {
				first = err
			}
			current.mu.Unlock()
			continue
		}
		delete(current.drafts, id)
		current.persisted[id] = struct{}{}
		current.updateSeq++
		seq := current.updateSeq
		current.mu.Unlock()
		out = append(out, persisted{entry: entry, after: after, seq: seq})
	}
	current.handoff.Unlock()

	for _, item := range out {
		entry := item.entry
		pubErr := r.publish(context.Background(), RunEvent{
			SessionID:     sessionID,
			RunID:         runID,
			Kind:          Message,
			EntryID:       entry.ID,
			AfterEntrySeq: item.after,
			Entry:         &entry,
			SeqEpoch:      r.epoch,
			UpdateSeq:     item.seq,
		})
		if pubErr != nil && first == nil {
			first = pubErr
		}
	}
	return first
}

func incompleteFromDraft(draft runDraft) session.Message {
	blocks := make([]session.Block, 0, len(draft.blocks))
	for _, block := range draft.blocks {
		if (block.Kind == "text" || block.Kind == "reasoning") && block.Text != "" {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return session.Message{}
	}
	return session.Message{Role: session.RoleAssistant, Blocks: blocks, Incomplete: true}
}

func (r *Runner) liveEvent(current *liveRun, event RunEvent) RunEvent {
	current.mu.Lock()
	defer current.mu.Unlock()
	// 结束状态和快照边界同一次提交，不能让快照提前吃掉结束事件。
	if event.Kind == RunEnded {
		current.ended = true
		current.endStatus = event.Status
		current.endError = event.Error
	}
	if event.Kind == ContextUsage {
		current.usage = cloneUsage(event.Usage)
	}
	event.SeqEpoch = r.epoch
	if event.UpdateSeq == 0 {
		current.updateSeq++
		event.UpdateSeq = current.updateSeq
	}
	return event
}

func (r *liveRun) markPersisted(id string) {
	r.mu.Lock()
	r.persisted[id] = struct{}{}
	r.mu.Unlock()
}

func mapEvent(sessionID, runID string, event loops.Event) (RunEvent, error) {
	out := RunEvent{SessionID: sessionID, RunID: runID, EntryID: event.EntryID, BlockSeq: event.BlockSeq}
	switch event.Kind {
	case loops.EventTextDelta:
		out.Kind = TextDelta
		out.Text = event.Text
	case loops.EventReasoningDelta:
		out.Kind = ReasoningDelta
		out.Text = event.Text
	case loops.EventToolStarted:
		out.Kind = ToolStarted
	case loops.EventToolFinished:
		out.Kind = ToolFinished
	case loops.EventUsage:
		out.Kind = ContextUsage
		if event.Usage != nil {
			usage := Usage{
				InputTokens:     event.Usage.InputTokens,
				CacheReadTokens: event.Usage.CacheReadTokens,
				ContextWindow:   event.Usage.ContextWindow,
			}
			out.Usage = &usage
		}
	default:
		return RunEvent{}, fmt.Errorf("runner: unsupported loop event %q", event.Kind)
	}
	if event.Tool != nil {
		out.Tool = &ToolEvent{ID: event.Tool.ID, Name: event.Tool.Name, IsError: event.Tool.IsError}
	}
	return out, nil
}

func applyDraftText(blocks []session.Block, kind, text string, blockSeq uint64) []session.Block {
	if blockSeq == 0 {
		return blocks
	}
	index := int(blockSeq - 1)
	for len(blocks) <= index {
		blocks = append(blocks, session.Block{})
	}
	if blocks[index].Kind == "" {
		blocks[index].Kind = kind
	}
	blocks[index].Text += text
	return blocks
}
