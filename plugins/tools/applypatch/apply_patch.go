package applypatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"harness/kernel/machine"
	"harness/kernel/tools"
)

// 数据。apply_patch 工具的模型参数。
type Args struct {
	Patch string `json:"patch" jsonschema:"minLength=1,description=Complete patch text using the *** Begin Patch and *** End Patch format."`
}

func newTool(files machine.FileSystem) tools.Tool {
	description := "Apply a Codex-format patch to add, update, or delete files. Begin with '*** Begin Patch' and end with '*** End Patch'. Use '*** Add File:', '*** Update File:', or '*** Delete File:' headers; update lines start with space, '+', or '-'. File moves are unsupported."
	return tools.New("apply_patch", description, func(ctx context.Context, call tools.Call, args Args) (tools.Result, error) {
		delta, summary, err := applyPatch(ctx, files, call.Workspace, args.Patch)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return tools.Result{}, ctxErr
		}
		if err != nil {
			content := err.Error()
			if len(delta.Changes) > 0 {
				content = formatPartialFailure(delta, err)
			}
			return tools.Result{Content: content, IsError: true}, nil
		}
		return tools.Result{Content: summary}, nil
	})
}

func applyPatch(ctx context.Context, files machine.FileSystem, workspace, patch string) (AppliedDelta, string, error) {
	hunks, err := parsePatch(patch)
	if err != nil {
		return AppliedDelta{Exact: true}, "", err
	}
	changes, err := prepareChanges(files, workspace, hunks)
	if err != nil {
		return AppliedDelta{Exact: true}, "", err
	}
	delta, err := commitChanges(ctx, files, changes)
	if err != nil {
		return delta, "", err
	}
	return delta, formatSuccess(changes), nil
}

func prepareChanges(files machine.FileSystem, workspace string, hunks []hunk) ([]preparedChange, error) {
	changes := make([]preparedChange, 0, len(hunks))
	seen := make(map[string]struct{}, len(hunks))
	for _, patchHunk := range hunks {
		path := files.ResolvePath(workspace, patchHunk.Path)
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("invalid patch: multiple operations target %s", path)
		}
		seen[key] = struct{}{}

		change := preparedChange{path: path, displayPath: patchHunk.Path, operation: patchHunk.Operation}
		switch patchHunk.Operation {
		case operationAdd:
			current, readErr := files.ReadFileVersion(path, 0)
			if readErr == nil {
				oldContent := string(current.Data)
				change.oldContent = &oldContent
				change.expectedHash = current.Hash
			} else if !errors.Is(readErr, os.ErrNotExist) {
				return nil, fmt.Errorf("failed to inspect file to add %s: %w", path, readErr)
			}
			newContent := patchHunk.Contents
			change.newContent = &newContent

		case operationDelete:
			current, readErr := files.ReadFileVersion(path, 0)
			if readErr != nil {
				return nil, fmt.Errorf("failed to read %s: %w", path, readErr)
			}
			oldContent := string(current.Data)
			change.expectedHash = current.Hash
			change.oldContent = &oldContent

		case operationUpdate:
			current, readErr := files.ReadFileVersion(path, 0)
			if readErr != nil {
				return nil, fmt.Errorf("failed to read file to update %s: %w", path, readErr)
			}
			oldContent := string(current.Data)
			newContent, updateErr := deriveNewContents(path, oldContent, patchHunk.Chunks)
			if updateErr != nil {
				return nil, updateErr
			}
			change.expectedHash = current.Hash
			change.oldContent = &oldContent
			change.newContent = &newContent
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func commitChanges(ctx context.Context, files machine.FileSystem, changes []preparedChange) (AppliedDelta, error) {
	delta := AppliedDelta{Exact: true}
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return delta, err
		}

		switch change.operation {
		case operationAdd, operationUpdate:
			_, err := files.WriteFileIfUnchanged(change.path, []byte(*change.newContent), change.expectedHash)
			if err != nil {
				delta.Exact = false
				return delta, fmt.Errorf("failed to write file %s: %w", change.path, err)
			}
		case operationDelete:
			err := files.RemoveFileIfUnchanged(change.path, change.expectedHash)
			if err != nil {
				current, readErr := files.ReadFile(change.path)
				if readErr != nil || change.oldContent == nil || string(current) != *change.oldContent {
					delta.Exact = false
				}
				return delta, fmt.Errorf("failed to delete file %s: %w", change.path, err)
			}
		}

		delta.Changes = append(delta.Changes, AppliedChange{
			Path:       change.path,
			Operation:  string(change.operation),
			OldContent: copyString(change.oldContent),
			NewContent: copyString(change.newContent),
		})
	}
	return delta, nil
}

func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func formatSuccess(changes []preparedChange) string {
	lines := []string{"Success. Updated the following files:"}
	for _, change := range changes {
		marker := "M"
		if change.operation == operationAdd {
			marker = "A"
		}
		if change.operation == operationDelete {
			marker = "D"
		}
		lines = append(lines, marker+" "+change.displayPath)
	}
	return strings.Join(lines, "\n")
}

func formatPartialFailure(delta AppliedDelta, err error) string {
	lines := []string{err.Error(), "", "Applied before failure:"}
	for _, change := range delta.Changes {
		marker := "M"
		if change.Operation == string(operationAdd) {
			marker = "A"
		}
		if change.Operation == string(operationDelete) {
			marker = "D"
		}
		lines = append(lines, marker+" "+change.Path)
	}
	return strings.Join(lines, "\n")
}
