package runner

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"harness/kernel/machine"
	"harness/kernel/persist"
	"harness/kernel/tools"
)

const runDiffVersion = 1

var (
	ErrRunDiffNotFound = errors.New("runner: run diff not found")
	ErrRunDiffConflict = errors.New("runner: run diff changed")
	ErrRunDiffActive   = errors.New("runner: run is still active")
)

type runDiffBody struct {
	Version  int                 `json:"version"`
	RunID    string              `json:"runID"`
	Revision uint64              `json:"revision"`
	Files    []trackedFileChange `json:"files"`
}

func (r *Runner) persistLiveDiff(sessionID, runID string, current *liveRun, snapshot turnDiffSnapshot, exact bool) error {
	current.mu.Lock()
	revision := uint64(1)
	if current.diffSummary != nil {
		revision = current.diffSummary.Revision + 1
	}
	current.mu.Unlock()

	var summary *RunDiffSummary
	if exact && len(snapshot.Files) > 0 {
		summary = summaryFromSnapshot(runID, revision, snapshot.Summaries)
	}

	r.recordsMu.Lock()
	err := r.storeDiffLocked(sessionID, runID, revision, snapshot.Files, summary)
	r.recordsMu.Unlock()
	if err != nil {
		return err
	}

	current.mu.Lock()
	current.diffSummary = cloneDiffSummary(summary)
	current.mu.Unlock()
	event := r.liveEvent(current, RunEvent{SessionID: sessionID, RunID: runID, Kind: RunDiffUpdated, Diff: cloneDiffSummary(summary)})
	return r.publish(context.Background(), event)
}

func (r *Runner) storeDiffLocked(sessionID, runID string, revision uint64, files []trackedFileChange, summary *RunDiffSummary) error {
	if r.files == nil {
		return fmt.Errorf("runner: diff storage is unavailable")
	}
	wasReconciled := r.reconciled[sessionID]
	if summary == nil {
		err := r.removeDiffBody(sessionID, runID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		err := r.writeDiffBody(sessionID, runDiffBody{Version: runDiffVersion, RunID: runID, Revision: revision, Files: files})
		if err != nil {
			return err
		}
	}

	records, err := r.loadRecords(sessionID)
	if err != nil {
		r.reconciled[sessionID] = false
		return err
	}
	found := false
	for index := range records {
		if records[index].RunID == runID {
			records[index].Diff = cloneDiffSummary(summary)
			found = true
			break
		}
	}
	if !found {
		r.reconciled[sessionID] = false
		return fmt.Errorf("runner: run record %q not found", runID)
	}
	err = r.saveRecords(sessionID, records)
	if err != nil {
		r.reconciled[sessionID] = false
		return err
	}
	// 本次正文与摘要已经一致；其他 Run 若尚未完成启动恢复，仍要保留待恢复状态。
	r.reconciled[sessionID] = wasReconciled
	return nil
}

func (r *Runner) ReadRunDiffFile(sessionID, runID, path string) (RunDiffFile, error) {
	if _, err := r.sessions.Get(sessionID); err != nil {
		return RunDiffFile{}, err
	}
	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()
	records, err := r.loadRecords(sessionID)
	if err != nil {
		return RunDiffFile{}, err
	}
	records, err = r.reconcileDiffsLocked(sessionID, records)
	if err != nil {
		return RunDiffFile{}, err
	}
	found := false
	for _, record := range records {
		if record.RunID == runID && record.Diff != nil {
			found = true
			break
		}
	}
	if !found {
		return RunDiffFile{}, ErrRunDiffNotFound
	}
	body, err := r.readDiffBody(sessionID, runID)
	if errors.Is(err, os.ErrNotExist) {
		return RunDiffFile{}, ErrRunDiffNotFound
	}
	if err != nil {
		return RunDiffFile{}, err
	}
	for _, file := range body.Files {
		if file.Path == path {
			return RunDiffFile{RunID: runID, Revision: body.Revision, Path: file.Path, Operation: file.Operation, OldContent: cloneText(file.OldContent), NewContent: cloneText(file.NewContent)}, nil
		}
	}
	return RunDiffFile{}, ErrRunDiffNotFound
}

func (r *Runner) RevertRunDiffFile(sessionID, runID, path string, expectedRevision uint64) (RunDiffSummary, error) {
	if _, err := r.sessions.Get(sessionID); err != nil {
		return RunDiffSummary{}, err
	}
	if r.filesystem == nil {
		return RunDiffSummary{}, fmt.Errorf("runner: filesystem is unavailable")
	}
	r.mu.Lock()
	_, active := r.live[sessionID]
	r.mu.Unlock()
	if active {
		return RunDiffSummary{}, ErrRunDiffActive
	}

	r.recordsMu.Lock()
	records, err := r.loadRecords(sessionID)
	if err != nil {
		r.recordsMu.Unlock()
		return RunDiffSummary{}, err
	}
	records, err = r.reconcileDiffsLocked(sessionID, records)
	if err != nil {
		r.recordsMu.Unlock()
		return RunDiffSummary{}, err
	}
	var record *runRecord
	for index := range records {
		if records[index].RunID == runID {
			record = &records[index]
			break
		}
	}
	if record == nil || record.Diff == nil {
		r.recordsMu.Unlock()
		return RunDiffSummary{}, ErrRunDiffNotFound
	}
	if record.Status == RunRunning {
		r.recordsMu.Unlock()
		return RunDiffSummary{}, ErrRunDiffActive
	}
	body, err := r.readDiffBody(sessionID, runID)
	if err != nil {
		r.recordsMu.Unlock()
		return RunDiffSummary{}, err
	}
	if body.Revision != expectedRevision {
		r.recordsMu.Unlock()
		return RunDiffSummary{}, ErrRunDiffConflict
	}

	index := -1
	for fileIndex := range body.Files {
		if body.Files[fileIndex].Path == path {
			index = fileIndex
			break
		}
	}
	if index < 0 {
		r.recordsMu.Unlock()
		return RunDiffSummary{}, ErrRunDiffNotFound
	}
	file := body.Files[index]
	if err = r.revertFile(file); err != nil {
		r.recordsMu.Unlock()
		if errors.Is(err, machine.ErrFileConflict) {
			return RunDiffSummary{}, fmt.Errorf("%w: %w", ErrRunDiffConflict, err)
		}
		return RunDiffSummary{}, err
	}

	body.Files = append(body.Files[:index], body.Files[index+1:]...)
	body.Revision++
	var summary *RunDiffSummary
	if len(body.Files) > 0 {
		summaries := make([]trackedFileSummary, 0, len(body.Files))
		deadline := time.Now().Add(diffStatsBudget)
		for _, remaining := range body.Files {
			additions, deletions := diffLineStats(remaining.OldContent, remaining.NewContent, deadline)
			summaries = append(summaries, trackedFileSummary{Path: remaining.Path, Operation: remaining.Operation, Additions: additions, Deletions: deletions})
		}
		summary = summaryFromSnapshot(runID, body.Revision, summaries)
	}
	err = r.storeDiffLocked(sessionID, runID, body.Revision, body.Files, summary)
	r.recordsMu.Unlock()
	if err != nil {
		return RunDiffSummary{}, err
	}
	event := r.nextSessionEvent(sessionID, RunEvent{SessionID: sessionID, RunID: runID, Kind: RunDiffUpdated, Diff: cloneDiffSummary(summary)})
	if err = r.publish(context.Background(), event); err != nil {
		return RunDiffSummary{}, err
	}
	if summary == nil {
		return RunDiffSummary{RunID: runID, Revision: body.Revision, Files: []FileDiffSummary{}}, nil
	}
	return *cloneDiffSummary(summary), nil
}

func (r *Runner) revertFile(file trackedFileChange) error {
	switch file.Operation {
	case tools.FileOperationAdd:
		return r.filesystem.RemoveFileIfUnchanged(file.Path, contentHash(file.NewContent))
	case tools.FileOperationUpdate:
		_, err := r.filesystem.WriteFileIfUnchanged(file.Path, []byte(*file.OldContent), contentHash(file.NewContent))
		return err
	case tools.FileOperationDelete:
		_, err := r.filesystem.WriteFileIfUnchanged(file.Path, []byte(*file.OldContent), machine.AbsentFileHash)
		return err
	default:
		return fmt.Errorf("runner: unsupported diff operation %q", file.Operation)
	}
}

func contentHash(content *string) string {
	if content == nil {
		return machine.AbsentFileHash
	}
	sum := sha256.Sum256([]byte(*content))
	return hex.EncodeToString(sum[:])
}

func (r *Runner) diffScope(sessionID string) (*persist.Files, error) {
	if r.files == nil {
		return nil, fmt.Errorf("runner: diff storage is unavailable")
	}
	return r.files.Scope("sessions", sessionID, "diffs")
}

func (r *Runner) writeDiffBody(sessionID string, body runDiffBody) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err = writer.Write(data); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	files, err := r.diffScope(sessionID)
	if err != nil {
		return err
	}
	return files.Write(body.RunID+".json.gz", compressed.Bytes())
}

func (r *Runner) readDiffBody(sessionID, runID string) (runDiffBody, error) {
	files, err := r.diffScope(sessionID)
	if err != nil {
		return runDiffBody{}, err
	}
	compressed, err := files.Read(runID + ".json.gz")
	if err != nil {
		return runDiffBody{}, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return runDiffBody{}, fmt.Errorf("runner: open diff %q: %w", runID, err)
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if err = errors.Join(readErr, closeErr); err != nil {
		return runDiffBody{}, fmt.Errorf("runner: read diff %q: %w", runID, err)
	}
	var body runDiffBody
	if err = json.Unmarshal(data, &body); err != nil {
		return runDiffBody{}, fmt.Errorf("runner: decode diff %q: %w", runID, err)
	}
	if body.Version != runDiffVersion || body.RunID != runID || body.Revision == 0 || len(body.Files) == 0 {
		return runDiffBody{}, fmt.Errorf("runner: invalid diff %q", runID)
	}
	previousPath := ""
	for _, file := range body.Files {
		if !validTrackedFile(file) {
			return runDiffBody{}, fmt.Errorf("runner: invalid diff file in %q", runID)
		}
		if previousPath != "" && file.Path <= previousPath {
			return runDiffBody{}, fmt.Errorf("runner: unordered or duplicate diff file in %q", runID)
		}
		previousPath = file.Path
	}
	return body, nil
}

func validTrackedFile(file trackedFileChange) bool {
	return validAppliedChange(tools.AppliedFileChange{Path: file.Path, Operation: file.Operation, OldContent: file.OldContent, NewContent: file.NewContent})
}

func (r *Runner) removeDiffBody(sessionID, runID string) error {
	files, err := r.diffScope(sessionID)
	if err != nil {
		return err
	}
	return files.Remove(runID + ".json.gz")
}

func (r *Runner) reconcileDiffsLocked(sessionID string, records []runRecord) ([]runRecord, error) {
	if r.files == nil || r.reconciled[sessionID] {
		return records, nil
	}
	changed := false
	for index := range records {
		body, err := r.readDiffBody(sessionID, records[index].RunID)
		if errors.Is(err, os.ErrNotExist) {
			if records[index].Diff != nil {
				records[index].Diff = nil
				changed = true
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		summary := summaryFromBody(body)
		if !reflect.DeepEqual(records[index].Diff, summary) {
			records[index].Diff = summary
			changed = true
		}
	}
	if changed {
		if err := r.saveRecords(sessionID, records); err != nil {
			return nil, err
		}
	}
	r.reconciled[sessionID] = true
	return records, nil
}

func summaryFromBody(body runDiffBody) *RunDiffSummary {
	summaries := make([]trackedFileSummary, 0, len(body.Files))
	deadline := time.Now().Add(diffStatsBudget)
	for _, file := range body.Files {
		additions, deletions := diffLineStats(file.OldContent, file.NewContent, deadline)
		summaries = append(summaries, trackedFileSummary{Path: file.Path, Operation: file.Operation, Additions: additions, Deletions: deletions})
	}
	return summaryFromSnapshot(body.RunID, body.Revision, summaries)
}

func summaryFromSnapshot(runID string, revision uint64, files []trackedFileSummary) *RunDiffSummary {
	out := &RunDiffSummary{RunID: runID, Revision: revision, Files: make([]FileDiffSummary, 0, len(files))}
	for _, file := range files {
		out.Files = append(out.Files, FileDiffSummary{Path: file.Path, Operation: file.Operation, Additions: file.Additions, Deletions: file.Deletions})
	}
	return out
}

func cloneDiffSummary(summary *RunDiffSummary) *RunDiffSummary {
	if summary == nil {
		return nil
	}
	copy := *summary
	copy.Files = append([]FileDiffSummary(nil), summary.Files...)
	return &copy
}

func (r *Runner) nextSessionEvent(sessionID string, event RunEvent) RunEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	event.SeqEpoch = r.epoch
	if current := r.live[sessionID]; current != nil {
		current.mu.Lock()
		current.updateSeq++
		event.UpdateSeq = current.updateSeq
		current.mu.Unlock()
		return event
	}
	r.clocks[sessionID]++
	event.UpdateSeq = r.clocks[sessionID]
	return event
}
