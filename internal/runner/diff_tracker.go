package runner

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"harness/internal/tools"
)

const diffStatsBudget = 100 * time.Millisecond

// 数据。一个文件在本轮中的完整净变化。
type trackedFileChange struct {
	Path       string              `json:"path"`
	Operation  tools.FileOperation `json:"operation"`
	OldContent *string             `json:"oldContent"`
	NewContent *string             `json:"newContent"`
}

// 数据。一个文件在本轮中的行数统计。
type trackedFileSummary struct {
	Path      string
	Operation tools.FileOperation
	Additions int
	Deletions int
}

// 数据。一轮当前可信的文件变化快照。
type turnDiffSnapshot struct {
	Files     []trackedFileChange
	Summaries []trackedFileSummary
}

type trackedFile struct {
	baseline *string
	current  *string
	stats    trackedFileSummary
}

// 活对象。聚合同一 Run 中 apply_patch 已经实际落盘的净变化。
type turnDiffTracker struct {
	exact bool
	files map[string]*trackedFile
}

func newTurnDiffTracker() *turnDiffTracker {
	return &turnDiffTracker{exact: true, files: make(map[string]*trackedFile)}
}

// Apply 接收一批真实落盘变化；来源一旦不可靠，本轮永久失效。
func (t *turnDiffTracker) Apply(delta tools.AppliedFileDelta) {
	if !t.exact {
		return
	}
	if !delta.Exact {
		t.invalidate()
		return
	}

	deadline := time.Now().Add(diffStatsBudget)
	for _, change := range delta.Changes {
		if !validAppliedChange(change) {
			t.invalidate()
			return
		}
		file := t.files[change.Path]
		if file == nil {
			file = &trackedFile{baseline: cloneText(change.OldContent)}
			t.files[change.Path] = file
		} else if !sameText(file.current, change.OldContent) {
			t.invalidate()
			return
		}
		file.current = cloneText(change.NewContent)
		if sameText(file.baseline, file.current) {
			delete(t.files, change.Path)
			continue
		}

		operation := netOperation(file.baseline, file.current)
		additions, deletions := diffLineStats(file.baseline, file.current, deadline)
		file.stats = trackedFileSummary{
			Path:      change.Path,
			Operation: operation,
			Additions: additions,
			Deletions: deletions,
		}
	}
}

// Snapshot 返回按路径稳定排序的副本；false 表示本轮已经失去可信来源。
func (t *turnDiffTracker) Snapshot() (turnDiffSnapshot, bool) {
	if !t.exact {
		return turnDiffSnapshot{}, false
	}
	paths := make([]string, 0, len(t.files))
	for path := range t.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	snapshot := turnDiffSnapshot{
		Files:     make([]trackedFileChange, 0, len(paths)),
		Summaries: make([]trackedFileSummary, 0, len(paths)),
	}
	for _, path := range paths {
		file := t.files[path]
		snapshot.Files = append(snapshot.Files, trackedFileChange{
			Path:       path,
			Operation:  file.stats.Operation,
			OldContent: cloneText(file.baseline),
			NewContent: cloneText(file.current),
		})
		snapshot.Summaries = append(snapshot.Summaries, file.stats)
	}
	return snapshot, true
}

func (t *turnDiffTracker) invalidate() {
	t.exact = false
	clear(t.files)
}

func validAppliedChange(change tools.AppliedFileChange) bool {
	if change.Path == "" || !validText(change.OldContent) || !validText(change.NewContent) {
		return false
	}
	switch change.Operation {
	case tools.FileOperationAdd:
		return change.OldContent == nil && change.NewContent != nil
	case tools.FileOperationUpdate:
		return change.OldContent != nil && change.NewContent != nil
	case tools.FileOperationDelete:
		return change.OldContent != nil && change.NewContent == nil
	default:
		return false
	}
}

func validText(content *string) bool {
	return content == nil || utf8.ValidString(*content)
}

func netOperation(oldContent, newContent *string) tools.FileOperation {
	if oldContent == nil {
		return tools.FileOperationAdd
	}
	if newContent == nil {
		return tools.FileOperationDelete
	}
	return tools.FileOperationUpdate
}

func sameText(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func cloneText(content *string) *string {
	if content == nil {
		return nil
	}
	copy := *content
	return &copy
}

func diffLineStats(oldContent, newContent *string, deadline time.Time) (int, int) {
	oldLines := contentLines(oldContent)
	newLines := contentLines(newContent)
	if len(oldLines) == 0 || len(newLines) == 0 {
		return len(newLines), len(oldLines)
	}
	// 过大的矩阵直接使用完整替换统计，避免病态文件占住 Runner。
	if len(oldLines) > 0 && len(newLines) > 4_000_000/len(oldLines) {
		return len(newLines), len(oldLines)
	}

	previous := make([]int, len(newLines)+1)
	current := make([]int, len(newLines)+1)
	steps := 0
	for _, oldLine := range oldLines {
		for newIndex, newLine := range newLines {
			if oldLine == newLine {
				current[newIndex+1] = previous[newIndex] + 1
			} else if previous[newIndex+1] >= current[newIndex] {
				current[newIndex+1] = previous[newIndex+1]
			} else {
				current[newIndex+1] = current[newIndex]
			}
			steps++
			if steps%1024 == 0 && time.Now().After(deadline) {
				return len(newLines), len(oldLines)
			}
		}
		previous, current = current, previous
		clear(current)
	}
	common := previous[len(newLines)]
	return len(newLines) - common, len(oldLines) - common
}

func contentLines(content *string) []string {
	if content == nil || *content == "" {
		return nil
	}
	lines := strings.Split(*content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
