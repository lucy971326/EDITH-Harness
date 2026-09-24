package applypatch

import (
	"fmt"
	"sort"
	"strings"
)

func deriveNewContents(path, original string, chunks []updateFileChunk) (string, error) {
	file := parseSourceFile(original)
	replacements, err := computeReplacements(file.lineTexts(), path, chunks)
	if err != nil {
		return "", err
	}
	file.applyReplacements(replacements)
	return file.contents(), nil
}

func computeReplacements(originalLines []string, path string, chunks []updateFileChunk) ([]replacement, error) {
	var replacements []replacement
	lineIndex := 0
	for _, chunk := range chunks {
		if chunk.ChangeContext != nil {
			index, found := seekSequence(originalLines, []string{*chunk.ChangeContext}, lineIndex, false)
			if !found {
				return nil, fmt.Errorf("failed to find context %q in %s", *chunk.ChangeContext, path)
			}
			lineIndex = index + 1
		}

		if len(chunk.OldLines) == 0 {
			replacements = append(replacements, replacement{start: len(originalLines), newLines: append([]string(nil), chunk.NewLines...)})
			continue
		}

		pattern := chunk.OldLines
		newLines := chunk.NewLines
		start, found := seekSequence(originalLines, pattern, lineIndex, chunk.IsEndOfFile)
		if !found && len(pattern) > 0 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
				newLines = newLines[:len(newLines)-1]
			}
			start, found = seekSequence(originalLines, pattern, lineIndex, chunk.IsEndOfFile)
		}
		if !found {
			return nil, fmt.Errorf("failed to find expected lines in %s:\n%s", path, joinLines(chunk.OldLines))
		}

		oldStart := 0
		newStart := 0
		for _, context := range chunk.ContextLineIndices {
			oldContext := context[0]
			newContext := context[1]
			if oldContext >= len(pattern) || newContext >= len(newLines) {
				break
			}
			if oldStart != oldContext || newStart != newContext {
				replacements = append(replacements, replacement{
					start:    start + oldStart,
					oldCount: oldContext - oldStart,
					newLines: append([]string(nil), newLines[newStart:newContext]...),
				})
			}
			oldStart = oldContext + 1
			newStart = newContext + 1
		}
		if oldStart != len(pattern) || newStart != len(newLines) {
			replacements = append(replacements, replacement{
				start:    start + oldStart,
				oldCount: len(pattern) - oldStart,
				newLines: append([]string(nil), newLines[newStart:]...),
			})
		}
		lineIndex = start + len(pattern)
	}
	sort.SliceStable(replacements, func(left, right int) bool {
		return replacements[left].start < replacements[right].start
	})
	return replacements, nil
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
