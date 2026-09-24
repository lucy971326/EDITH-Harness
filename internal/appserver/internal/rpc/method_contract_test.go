package rpc

import (
	"context"
	"encoding/json"
	"testing"

	"harness/internal/runner"
	"harness/internal/tools"
)

func TestRunDiffFileContractAcceptsEveryFileOperation(t *testing.T) {
	oldContent := "before"
	newContent := "after"
	tests := []struct {
		name      string
		operation tools.FileOperation
		old       *string
		new       *string
	}{
		{name: "add", operation: tools.FileOperationAdd, new: &newContent},
		{name: "update", operation: tools.FileOperationUpdate, old: &oldContent, new: &newContent},
		{name: "delete", operation: tools.FileOperationDelete, old: &oldContent},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method, err := bind("test/run/diff/read", func(context.Context, struct{}) (runner.RunDiffFile, error) {
				return runner.RunDiffFile{
					RunID:      "run-1",
					Revision:   1,
					Path:       "file.txt",
					Operation:  test.operation,
					OldContent: test.old,
					NewContent: test.new,
				}, nil
			}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}

			result, err := method(context.Background(), json.RawMessage(`{}`))()
			if err != nil {
				t.Fatalf("Diff output rejected: %v", err)
			}
			var decoded runner.RunDiffFile
			if err := json.Unmarshal(result, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Operation != test.operation || (decoded.OldContent == nil) != (test.old == nil) || (decoded.NewContent == nil) != (test.new == nil) {
				t.Fatalf("decoded=%+v", decoded)
			}
		})
	}
}
