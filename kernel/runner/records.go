package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// 数据。磁盘上的一轮运行结果；不重复保存消息正文。
type runRecord struct {
	RunID         string    `json:"runID"`
	Status        RunStatus `json:"status"`
	AfterEntrySeq uint64    `json:"afterEntrySeq"`
	Error         string    `json:"error,omitempty"`
	Usage         *Usage    `json:"usage,omitempty"`
}

func (r *Runner) loadRecords(sessionID string) ([]runRecord, error) {
	body, err := r.persist.LoadRunRecords(sessionID)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []runRecord
	err = json.Unmarshal(body, &records)
	if err != nil {
		return nil, fmt.Errorf("runner: load run records %q: %w", sessionID, err)
	}
	if records == nil {
		records = []runRecord{}
	}
	return records, nil
}

func (r *Runner) saveRecords(sessionID string, records []runRecord) error {
	if records == nil {
		records = []runRecord{}
	}
	body, err := json.Marshal(records)
	if err != nil {
		return err
	}
	err = r.persist.SaveRunRecords(sessionID, body)
	if err != nil {
		return fmt.Errorf("runner: save run records %q: %w", sessionID, err)
	}
	return nil
}

func (r *Runner) upsertRecord(sessionID string, rec runRecord) error {
	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()
	records, err := r.loadRecords(sessionID)
	if err != nil {
		return err
	}
	replaced := false
	for i, existing := range records {
		if existing.RunID != rec.RunID {
			continue
		}
		records[i] = rec
		replaced = true
		break
	}
	if !replaced {
		records = append(records, rec)
	}
	return r.saveRecords(sessionID, records)
}

func (r *Runner) interruptStaleRecords(sessionID string, records []runRecord, live *liveRun) ([]runRecord, error) {
	changed := false
	liveID := ""
	if live != nil {
		live.mu.Lock()
		liveID = live.runID
		live.mu.Unlock()
	}
	for i, rec := range records {
		if rec.Status != RunRunning {
			continue
		}
		if liveID == rec.RunID {
			continue
		}
		records[i].Status = RunInterrupted
		records[i].Error = "interrupted by restart"
		changed = true
	}
	if !changed {
		return records, nil
	}
	err := r.saveRecords(sessionID, records)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (r *Runner) CopyRecordsForFork(sourceID, destID string, seqMap map[uint64]uint64) error {
	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()
	records, err := r.loadRecords(sourceID)
	if err != nil {
		return err
	}
	copied := make([]runRecord, 0, len(records))
	for _, rec := range records {
		after, ok := seqMap[rec.AfterEntrySeq]
		if !ok {
			continue
		}
		rec.AfterEntrySeq = after
		if rec.Status == RunRunning {
			rec.Status = RunInterrupted
			rec.Error = "interrupted by restart"
		}
		copied = append(copied, rec)
	}
	if len(copied) == 0 {
		return nil
	}
	return r.saveRecords(destID, copied)
}
