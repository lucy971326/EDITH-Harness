package applypatch

import (
	"fmt"
	"strings"
)

const (
	beginPatchMarker    = "*** Begin Patch"
	endPatchMarker      = "*** End Patch"
	addFileMarker       = "*** Add File: "
	deleteFileMarker    = "*** Delete File: "
	updateFileMarker    = "*** Update File: "
	moveToMarker        = "*** Move to: "
	endOfFileMarker     = "*** End of File"
	changeContextMarker = "@@ "
)

func parsePatch(patch string) ([]hunk, error) {
	lines := patchLines(patch)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != beginPatchMarker {
		return nil, fmt.Errorf("invalid patch: the first line must be %q", beginPatchMarker)
	}
	if strings.TrimSpace(lines[len(lines)-1]) != endPatchMarker {
		return nil, fmt.Errorf("invalid patch: the last line must be %q", endPatchMarker)
	}

	var hunks []hunk
	for lineIndex := 1; lineIndex < len(lines)-1; {
		line := strings.TrimSpace(lines[lineIndex])
		switch {
		case strings.HasPrefix(line, addFileMarker):
			path := strings.TrimSpace(strings.TrimPrefix(line, addFileMarker))
			if path == "" {
				return nil, invalidHunk(lineIndex+1, "add file path is empty")
			}
			lineIndex++
			var contents strings.Builder
			added := false
			for lineIndex < len(lines)-1 && !isHunkHeader(lines[lineIndex]) {
				text, ok := strings.CutPrefix(lines[lineIndex], "+")
				if !ok {
					return nil, invalidHunk(lineIndex+1, "every added file line must start with '+'")
				}
				contents.WriteString(text)
				contents.WriteByte('\n')
				added = true
				lineIndex++
			}
			if !added {
				return nil, invalidHunk(lineIndex+1, "add file hunk is empty")
			}
			hunks = append(hunks, hunk{Operation: operationAdd, Path: path, Contents: contents.String()})

		case strings.HasPrefix(line, deleteFileMarker):
			path := strings.TrimSpace(strings.TrimPrefix(line, deleteFileMarker))
			if path == "" {
				return nil, invalidHunk(lineIndex+1, "delete file path is empty")
			}
			hunks = append(hunks, hunk{Operation: operationDelete, Path: path})
			lineIndex++
			if lineIndex < len(lines)-1 && !isHunkHeader(lines[lineIndex]) {
				return nil, invalidHunk(lineIndex+1, "delete file hunk cannot contain body lines")
			}

		case strings.HasPrefix(line, updateFileMarker):
			path := strings.TrimSpace(strings.TrimPrefix(line, updateFileMarker))
			if path == "" {
				return nil, invalidHunk(lineIndex+1, "update file path is empty")
			}
			lineIndex++
			chunks, next, err := parseUpdate(lines, lineIndex)
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, hunk{Operation: operationUpdate, Path: path, Chunks: chunks})
			lineIndex = next

		default:
			return nil, invalidHunk(lineIndex+1, fmt.Sprintf("%q is not a valid hunk header", line))
		}
	}
	if len(hunks) == 0 {
		return nil, fmt.Errorf("invalid patch: no files were modified")
	}
	return hunks, nil
}

func patchLines(patch string) []string {
	trimmed := strings.TrimSpace(patch)
	lines := strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n")
	if len(lines) >= 4 {
		first := lines[0]
		last := lines[len(lines)-1]
		if (first == "<<EOF" || first == "<<'EOF'" || first == "<<\"EOF\"") && strings.HasSuffix(last, "EOF") {
			lines = lines[1 : len(lines)-1]
		}
	}
	return lines
}

func parseUpdate(lines []string, start int) ([]updateFileChunk, int, error) {
	var chunks []updateFileChunk
	for lineIndex := start; lineIndex < len(lines)-1 && !isUpdateBoundary(lines[lineIndex]); lineIndex++ {
		line := strings.TrimSuffix(lines[lineIndex], "\r")
		trimmedEnd := strings.TrimRight(line, " \t")

		if strings.HasPrefix(trimmedEnd, moveToMarker) {
			return nil, 0, invalidHunk(lineIndex+1, "file moves are not supported")
		}
		if len(chunks) > 0 && chunks[len(chunks)-1].IsEndOfFile {
			if trimmedEnd == "" {
				continue
			}
			if !isContextMarker(trimmedEnd) {
				return nil, 0, invalidHunk(lineIndex+1, "expected a new @@ context marker after end of file")
			}
		}

		switch {
		case isContextMarker(trimmedEnd):
			if len(chunks) > 0 && chunkEmpty(chunks[len(chunks)-1]) {
				return nil, 0, invalidHunk(lineIndex+1, "update hunk does not contain any lines")
			}
			chunk := updateFileChunk{}
			if trimmedEnd != "@@" {
				context := strings.TrimPrefix(trimmedEnd, changeContextMarker)
				chunk.ChangeContext = &context
			}
			chunks = append(chunks, chunk)

		case trimmedEnd == endOfFileMarker:
			if len(chunks) == 0 || chunkEmpty(chunks[len(chunks)-1]) {
				return nil, 0, invalidHunk(lineIndex+1, "update hunk does not contain any lines")
			}
			chunks[len(chunks)-1].IsEndOfFile = true

		case line == "":
			chunks = ensureChunk(chunks)
			chunks[len(chunks)-1].pushContextLine("")

		case strings.HasPrefix(line, " "):
			chunks = ensureChunk(chunks)
			chunks[len(chunks)-1].pushContextLine(line[1:])

		case strings.HasPrefix(line, "+"):
			chunks = ensureChunk(chunks)
			chunks[len(chunks)-1].NewLines = append(chunks[len(chunks)-1].NewLines, line[1:])

		case strings.HasPrefix(line, "-"):
			chunks = ensureChunk(chunks)
			chunks[len(chunks)-1].OldLines = append(chunks[len(chunks)-1].OldLines, line[1:])

		default:
			return nil, 0, invalidHunk(lineIndex+1, "every update line must start with ' ', '+', '-', or '@@'")
		}
	}

	next := start
	for next < len(lines)-1 && !isUpdateBoundary(lines[next]) {
		next++
	}
	if len(chunks) == 0 || chunkEmpty(chunks[len(chunks)-1]) {
		return nil, 0, invalidHunk(start+1, "update file hunk is empty")
	}
	return chunks, next, nil
}

func ensureChunk(chunks []updateFileChunk) []updateFileChunk {
	if len(chunks) == 0 {
		return append(chunks, updateFileChunk{})
	}
	return chunks
}

func chunkEmpty(chunk updateFileChunk) bool {
	return len(chunk.OldLines) == 0 && len(chunk.NewLines) == 0
}

func isContextMarker(line string) bool {
	return line == "@@" || strings.HasPrefix(line, changeContextMarker)
}

func isHunkHeader(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == endPatchMarker || strings.HasPrefix(trimmed, addFileMarker) || strings.HasPrefix(trimmed, deleteFileMarker) || strings.HasPrefix(trimmed, updateFileMarker)
}

func isUpdateBoundary(line string) bool {
	trimmedEnd := strings.TrimRight(line, " \t\r")
	return trimmedEnd == endPatchMarker || strings.HasPrefix(trimmedEnd, addFileMarker) || strings.HasPrefix(trimmedEnd, deleteFileMarker) || strings.HasPrefix(trimmedEnd, updateFileMarker)
}

func invalidHunk(line int, message string) error {
	return fmt.Errorf("invalid patch hunk on line %d: %s", line, message)
}
