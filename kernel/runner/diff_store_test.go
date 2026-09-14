package runner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"harness/kernel/host"
	"harness/kernel/machine"
	"harness/kernel/persist"
	"harness/kernel/tools"
	machinelocal "harness/plugins/machine/local"
)

func TestRunDiffRepairsSummaryAndCopiesBodyOnFork(t *testing.T) {
	fixture := newRunnerFixture(t, &runnerTestLoop{})
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture.runner.diffFiles = files
	oldText, newText := "old\n", "new\n"
	body := runDiffBody{Version: runDiffVersion, RunID: "run-a", Revision: 2, Files: []trackedFileChange{{Path: "main.go", Operation: tools.FileOperationUpdate, OldContent: &oldText, NewContent: &newText}}}
	if err = fixture.runner.upsertRecord("session-1", runRecord{RunID: "run-a", Status: RunSucceeded, AfterEntrySeq: 1, Diff: &RunDiffSummary{RunID: "run-a", Revision: 1}}); err != nil {
		t.Fatal(err)
	}
	if err = fixture.runner.writeDiffBody("session-1", body); err != nil {
		t.Fatal(err)
	}

	view, err := fixture.runner.SessionView("session-1")
	if err != nil || view.Runs[0].Diff == nil || view.Runs[0].Diff.Revision != 2 {
		t.Fatalf("repaired diff = %#v, %v", view.Runs, err)
	}
	if err = fixture.runner.CopyRecordsForFork("session-1", "fork-1", map[uint64]uint64{1: 1}); err != nil {
		t.Fatal(err)
	}
	copied, err := fixture.runner.readDiffBody("fork-1", "run-a")
	if err != nil || copied.Revision != 2 || len(copied.Files) != 1 {
		t.Fatalf("copied body = %#v, %v", copied, err)
	}
}

func TestRunDiffRevertIsConflictSafe(t *testing.T) {
	fixture := newRunnerFixture(t, &runnerTestLoop{})
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture.runner.diffFiles = files
	fixture.runner.filesystem = localTestMachine(t)

	path := filepath.Join(t.TempDir(), "main.go")
	oldText, newText := "old\n", "new\n"
	if err = os.WriteFile(path, []byte(newText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = fixture.runner.upsertRecord("session-1", runRecord{RunID: "run-a", Status: RunSucceeded, AfterEntrySeq: 1}); err != nil {
		t.Fatal(err)
	}
	snapshot := turnDiffSnapshot{
		Files:     []trackedFileChange{{Path: path, Operation: tools.FileOperationUpdate, OldContent: &oldText, NewContent: &newText}},
		Summaries: []trackedFileSummary{{Path: path, Operation: tools.FileOperationUpdate, Additions: 1, Deletions: 1}},
	}
	summary := summaryFromSnapshot("run-a", 1, snapshot.Summaries)
	fixture.runner.recordsMu.Lock()
	err = fixture.runner.storeDiffLocked("session-1", "run-a", 1, snapshot.Files, summary)
	fixture.runner.recordsMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	result, err := fixture.runner.RevertRunDiffFile("session-1", "run-a", path, 1)
	if err != nil || len(result.Files) != 0 {
		t.Fatalf("revert = %#v, %v", result, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != oldText {
		t.Fatalf("file = %q, %v", data, err)
	}

	if err = os.WriteFile(path, []byte(newText), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.runner.recordsMu.Lock()
	err = fixture.runner.storeDiffLocked("session-1", "run-a", 3, snapshot.Files, summaryFromSnapshot("run-a", 3, snapshot.Summaries))
	fixture.runner.recordsMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.runner.RevertRunDiffFile("session-1", "run-a", path, 3)
	if !errors.Is(err, ErrRunDiffConflict) {
		t.Fatalf("conflict = %v", err)
	}
}

func localTestMachine(t *testing.T) machine.FileSystem {
	t.Helper()
	h := host.NewHost()
	if err := h.Install(machinelocal.New()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	filesystem, err := host.Resolve[machine.FileSystem](h, "machine")
	if err != nil {
		t.Fatal(err)
	}
	return filesystem
}
