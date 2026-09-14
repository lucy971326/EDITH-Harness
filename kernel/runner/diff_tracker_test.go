package runner

import (
	"testing"

	"harness/kernel/tools"
)

func TestTurnDiffTrackerKeepsNetChangeAndStableOrder(t *testing.T) {
	tracker := newTurnDiffTracker()
	before := "one\ntwo\n"
	middle := "one\nTWO\n"
	after := "one\nTWO\nthree\n"
	added := "new\n"
	tracker.Apply(tools.AppliedFileDelta{Exact: true, Changes: []tools.AppliedFileChange{
		{Path: "/z.txt", Operation: tools.FileOperationUpdate, OldContent: &before, NewContent: &middle},
		{Path: "/a.txt", Operation: tools.FileOperationAdd, NewContent: &added},
	}})
	tracker.Apply(tools.AppliedFileDelta{Exact: true, Changes: []tools.AppliedFileChange{
		{Path: "/z.txt", Operation: tools.FileOperationUpdate, OldContent: &middle, NewContent: &after},
	}})

	snapshot, exact := tracker.Snapshot()
	if !exact || len(snapshot.Files) != 2 || snapshot.Files[0].Path != "/a.txt" || snapshot.Files[1].Path != "/z.txt" {
		t.Fatalf("snapshot = %#v, exact = %v", snapshot, exact)
	}
	if *snapshot.Files[1].OldContent != before || *snapshot.Files[1].NewContent != after {
		t.Fatalf("net change = %#v", snapshot.Files[1])
	}
	stats := snapshot.Summaries[1]
	if stats.Additions != 2 || stats.Deletions != 1 {
		t.Fatalf("stats = %#v", stats)
	}
}

func TestTurnDiffTrackerRemovesRevertedChangeAndInvalidatesPermanently(t *testing.T) {
	tracker := newTurnDiffTracker()
	before := "before\n"
	after := "after\n"
	tracker.Apply(tools.AppliedFileDelta{Exact: true, Changes: []tools.AppliedFileChange{{
		Path: "/file.txt", Operation: tools.FileOperationUpdate, OldContent: &before, NewContent: &after,
	}}})
	tracker.Apply(tools.AppliedFileDelta{Exact: true, Changes: []tools.AppliedFileChange{{
		Path: "/file.txt", Operation: tools.FileOperationUpdate, OldContent: &after, NewContent: &before,
	}}})
	snapshot, exact := tracker.Snapshot()
	if !exact || len(snapshot.Files) != 0 {
		t.Fatalf("reverted snapshot = %#v, exact = %v", snapshot, exact)
	}

	tracker.Apply(tools.AppliedFileDelta{Exact: false})
	tracker.Apply(tools.AppliedFileDelta{Exact: true, Changes: []tools.AppliedFileChange{{
		Path: "/new.txt", Operation: tools.FileOperationAdd, NewContent: &after,
	}}})
	if _, exact := tracker.Snapshot(); exact {
		t.Fatal("tracker recovered after an inexact delta")
	}
}
