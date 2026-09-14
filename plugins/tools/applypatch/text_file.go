package applypatch

type sourceLine struct {
	text   string
	ending string
}

type sourceFile struct {
	lines           []sourceLine
	preferredEnding string
}

type replacement struct {
	start    int
	oldCount int
	newLines []string
}

func parseSourceFile(contents string) sourceFile {
	file := sourceFile{preferredEnding: "\n"}
	preferredSet := false
	lineStart := 0
	for cursor := 0; cursor < len(contents); {
		ending := ""
		endingLength := 0
		switch contents[cursor] {
		case '\r':
			ending = "\r"
			endingLength = 1
			if cursor+1 < len(contents) && contents[cursor+1] == '\n' {
				ending = "\r\n"
				endingLength = 2
			}
		case '\n':
			ending = "\n"
			endingLength = 1
		default:
			cursor++
			continue
		}
		if !preferredSet {
			file.preferredEnding = ending
			preferredSet = true
		}
		file.lines = append(file.lines, sourceLine{text: contents[lineStart:cursor], ending: ending})
		cursor += endingLength
		lineStart = cursor
	}
	if lineStart < len(contents) {
		file.lines = append(file.lines, sourceLine{text: contents[lineStart:]})
	}
	return file
}

func (file sourceFile) lineTexts() []string {
	lines := make([]string, len(file.lines))
	for index, line := range file.lines {
		lines[index] = line.text
	}
	return lines
}

func (file *sourceFile) applyReplacements(replacements []replacement) {
	newLines := make([]sourceLine, 0, len(file.lines))
	sourceIndex := 0
	for _, change := range replacements {
		newLines = append(newLines, file.lines[sourceIndex:change.start]...)
		for _, text := range change.newLines {
			newLines = append(newLines, sourceLine{text: text, ending: file.preferredEnding})
		}
		sourceIndex = change.start + change.oldCount
	}
	newLines = append(newLines, file.lines[sourceIndex:]...)
	for index := range newLines {
		if newLines[index].ending == "" {
			newLines[index].ending = file.preferredEnding
		}
	}
	file.lines = newLines
}

func (file sourceFile) contents() string {
	var contents []byte
	for _, line := range file.lines {
		contents = append(contents, line.text...)
		contents = append(contents, line.ending...)
	}
	return string(contents)
}
