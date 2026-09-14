package applypatch

type operation string

const (
	operationAdd    operation = "add"
	operationUpdate operation = "update"
	operationDelete operation = "delete"
)

type hunk struct {
	Operation operation
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

// 数据。一次 apply_patch 已经实际提交的文件变化。
type AppliedDelta struct {
	Changes []AppliedChange
	Exact   bool
}

// 数据。一个文件已经实际提交的旧内容与新内容。
type AppliedChange struct {
	Path       string
	Operation  string
	OldContent *string
	NewContent *string
}

type preparedChange struct {
	path         string
	displayPath  string
	operation    operation
	expectedHash string
	oldContent   *string
	newContent   *string
}
