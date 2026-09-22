package applypatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"unicode/utf8"

	"harness/kernel/machine"
	"harness/kernel/permissions"
	"harness/kernel/tools"
)

// 数据。apply_patch 工具的模型参数。
type Args struct {
	Patch string `json:"patch" jsonschema:"minLength=1,description=Complete patch text using the *** Begin Patch and *** End Patch format."`
}

func newTool(files machine.FileSystem, agent machine.AgentFiles) tools.Tool {
	description := "Create, edit, or delete files with a Codex-format patch. Always use this tool for deliberate file changes so Harness can track and safely revert them; do not edit files through exec_command. Begin with '*** Begin Patch' and end with '*** End Patch'. Use '*** Add File:', '*** Update File:', or '*** Delete File:' headers; update lines start with space, '+', or '-'. File moves are unsupported."
	return tools.New("apply_patch", description, func(ctx context.Context, call tools.Call, args Args) (tools.Result, error) {
		delta, summary, err := applyPatch(ctx, files, agent, call.Policy, call.Workspace, args.Patch)
		if ctxErr := ctx.Err(); ctxErr != nil {
			if len(delta.Changes) > 0 || !delta.Exact {
				content := ctxErr.Error()
				if err != nil {
					content = err.Error()
				}
				if len(delta.Changes) > 0 {
					content = formatPartialFailure(delta, errors.New(content))
				}
				return tools.Result{Content: content, IsError: true, FileDelta: &delta}, nil
			}
			return tools.Result{}, ctxErr
		}
		if err != nil {
			content := err.Error()
			if len(delta.Changes) > 0 {
				content = formatPartialFailure(delta, err)
			}
			return tools.Result{Content: content, IsError: true, FileDelta: &delta}, nil
		}
		return tools.Result{Content: summary, FileDelta: &delta}, nil
	})
}

func applyPatch(ctx context.Context, files machine.FileSystem, agent machine.AgentFiles, policy permissions.Policy, workspace, patch string) (tools.AppliedFileDelta, string, error) {
	if !utf8.ValidString(patch) {
		return tools.AppliedFileDelta{Exact: true}, "", fmt.Errorf("invalid patch: content is not valid UTF-8")
	}
	hunks, err := parsePatch(patch)
	if err != nil {
		return tools.AppliedFileDelta{Exact: true}, "", err
	}
	changes, err := prepareChanges(files, workspace, hunks)
	if err != nil {
		return tools.AppliedFileDelta{Exact: true}, "", err
	}
	delta, err := commitAgentChanges(ctx, agent, policy, changes)
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
		case tools.FileOperationAdd:
			current, readErr := files.ReadFileVersion(path, 0)
			if readErr == nil {
				if !utf8.Valid(current.Data) {
					return nil, fmt.Errorf("failed to add %s: existing file is not valid UTF-8", path)
				}
				oldContent := string(current.Data)
				change.oldContent = &oldContent
				change.expectedHash = current.Hash
				// Add File 覆盖已有文件时，记录真实发生的 update。
				change.operation = tools.FileOperationUpdate
			} else if !errors.Is(readErr, os.ErrNotExist) {
				return nil, fmt.Errorf("failed to inspect file to add %s: %w", path, readErr)
			}
			newContent := patchHunk.Contents
			if !utf8.ValidString(newContent) {
				return nil, fmt.Errorf("failed to add %s: new content is not valid UTF-8", path)
			}
			change.newContent = &newContent

		case tools.FileOperationDelete:
			current, readErr := files.ReadFileVersion(path, 0)
			if readErr != nil {
				return nil, fmt.Errorf("failed to read %s: %w", path, readErr)
			}
			if !utf8.Valid(current.Data) {
				return nil, fmt.Errorf("failed to delete %s: file is not valid UTF-8", path)
			}
			oldContent := string(current.Data)
			change.expectedHash = current.Hash
			change.oldContent = &oldContent

		case tools.FileOperationUpdate:
			current, readErr := files.ReadFileVersion(path, 0)
			if readErr != nil {
				return nil, fmt.Errorf("failed to read file to update %s: %w", path, readErr)
			}
			if !utf8.Valid(current.Data) {
				return nil, fmt.Errorf("failed to update %s: file is not valid UTF-8", path)
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

func commitAgentChanges(ctx context.Context, agent machine.AgentFiles, policy permissions.Policy, changes []preparedChange) (tools.AppliedFileDelta, error) {
	batch := make([]machine.FileChange, 0, len(changes))
	for _, change := range changes {
		batch = append(batch, machine.FileChange{Path: change.path, ExpectedHash: change.expectedHash, Content: change.newContent})
	}
	result, err := agent.AgentApplyChanges(ctx, policy, batch)
	delta := tools.AppliedFileDelta{Exact: result.Exact}
	for _, change := range changes[:result.Completed] {
		delta.Changes = append(delta.Changes, tools.AppliedFileChange{
			Path:       change.path,
			Operation:  change.operation,
			OldContent: copyString(change.oldContent),
			NewContent: copyString(change.newContent),
		})
	}
	return delta, err
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
		if change.operation == tools.FileOperationAdd {
			marker = "A"
		}
		if change.operation == tools.FileOperationDelete {
			marker = "D"
		}
		lines = append(lines, marker+" "+change.displayPath)
	}
	return strings.Join(lines, "\n")
}

func formatPartialFailure(delta tools.AppliedFileDelta, err error) string {
	lines := []string{err.Error(), "", "Applied before failure:"}
	for _, change := range delta.Changes {
		marker := "M"
		if change.Operation == tools.FileOperationAdd {
			marker = "A"
		}
		if change.Operation == tools.FileOperationDelete {
			marker = "D"
		}
		lines = append(lines, marker+" "+change.Path)
	}
	return strings.Join(lines, "\n")
}
