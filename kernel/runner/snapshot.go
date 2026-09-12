package runner

import "harness/kernel/session"

// SessionView 返回一本会话的账本、运行状态和更新边界；草稿是副本。
func (r *Runner) SessionView(sessionID string) (SessionView, error) {
	sess, err := r.sessions.Get(sessionID)
	if err != nil {
		return SessionView{}, err
	}
	// 固定运行身份及磁盘记录，避免旧快照覆盖刚写好的结束状态。
	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.live[sessionID]
	clock := r.clocks[sessionID]
	records, err := r.loadRecords(sessionID)
	if err != nil {
		return SessionView{}, err
	}

	records, err = r.interruptStaleRecords(sessionID, records, current)
	if err != nil {
		return SessionView{}, err
	}

	var entries []session.Entry
	var liveState RunState
	var hasLive bool
	updateSeq := uint64(0)
	if current != nil {
		current.handoff.Lock()
		current.mu.Lock()
		updateSeq = current.updateSeq
		if current.afterEntrySeq != 0 {
			liveState = current.snapshotLocked()
			hasLive = true
		}
		current.mu.Unlock()
		entries = sess.Entries()
		current.handoff.Unlock()
	} else {
		entries = sess.Entries()
		updateSeq = clock
	}

	runs := make([]RunState, 0, len(records)+1)
	seen := make(map[string]int, len(records)+1)
	for _, rec := range records {
		state := RunState{RunID: rec.RunID, AfterEntrySeq: rec.AfterEntrySeq, Status: rec.Status, Error: rec.Error, Usage: cloneUsage(rec.Usage)}
		seen[rec.RunID] = len(runs)
		runs = append(runs, state)
	}
	if hasLive {
		if index, ok := seen[liveState.RunID]; ok {
			liveState.AfterEntrySeq = maxSeq(liveState.AfterEntrySeq, runs[index].AfterEntrySeq)
			if liveState.Status == "" {
				liveState.Status = runs[index].Status
			}
			if liveState.Error == "" {
				liveState.Error = runs[index].Error
			}
			if liveState.Usage == nil {
				liveState.Usage = cloneUsage(runs[index].Usage)
			}
			runs[index] = liveState
		} else {
			runs = append(runs, liveState)
		}
	}
	if runs == nil {
		runs = []RunState{}
	}
	return SessionView{Entries: entries, Runs: runs, UpdateSeq: updateSeq, SeqEpoch: r.epoch}, nil
}

func (r *liveRun) snapshotLocked() RunState {
	status := RunRunning
	errText := ""
	if r.ended {
		status = r.endStatus
		errText = r.endError
		if status == "" {
			status = RunSucceeded
		}
	}
	drafts := make([]RunDraft, 0, len(r.drafts))
	for _, draft := range r.drafts {
		if draft == nil || !draftHasContent(draft.blocks) {
			continue
		}
		drafts = append(drafts, RunDraft{
			EntryID:       draft.entryID,
			AfterEntrySeq: draft.afterEntrySeq,
			Blocks:        cloneBlocks(draft.blocks),
		})
	}
	if len(drafts) == 0 {
		drafts = nil
	}
	return RunState{
		RunID:         r.runID,
		AfterEntrySeq: r.afterEntrySeq,
		Status:        status,
		Error:         errText,
		Drafts:        drafts,
		Usage:         cloneUsage(r.usage),
	}
}

func draftHasContent(blocks []session.Block) bool {
	for _, block := range blocks {
		if block.Text != "" || block.Tool != nil {
			return true
		}
	}
	return false
}

func maxSeq(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
