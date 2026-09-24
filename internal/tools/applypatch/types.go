package applypatch

import "harness/internal/tools"

type hunk struct {
	Operation tools.FileOperation
	Path      string
	Contents  string
	Chunks    []updateFileChunk
}

type updateFileChunk struct {
	ChangeContext      *string
	OldLines           []string
	NewLines           []string
	ContextLineIndices [][2]int
	IsEndOfFile        bool
}

func (chunk *updateFileChunk) pushContextLine(line string) {
	chunk.ContextLineIndices = append(chunk.ContextLineIndices, [2]int{len(chunk.OldLines), len(chunk.NewLines)})
	chunk.OldLines = append(chunk.OldLines, line)
	chunk.NewLines = append(chunk.NewLines, line)
}

type preparedChange struct {
	path         string
	displayPath  string
	operation    tools.FileOperation
	expectedHash string
	oldContent   *string
	newContent   *string
}
