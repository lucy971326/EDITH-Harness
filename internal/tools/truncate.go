package tools

import (
	"unicode/utf8"
)

const (
	maxOutputBytes = 50 * 1024
	maxOutputLines = 2000
)

const truncatedNotice = "\n\n[output truncated: showing at most 2000 lines or 50 KiB]"

// TruncateHead 保留文本开头，限制 MCP 工具结果大小。
func TruncateHead(text string) string {
	limited := firstLines(text)
	limited = firstBytes(limited)
	if limited == text {
		return text
	}
	return limited + truncatedNotice
}

func firstLines(text string) string {
	lines := 0
	for i := range len(text) {
		if text[i] != '\n' {
			continue
		}
		lines++
		if lines == maxOutputLines && i+1 < len(text) {
			return text[:i+1]
		}
	}
	return text
}

func firstBytes(text string) string {
	if len(text) <= maxOutputBytes {
		return text
	}
	end := maxOutputBytes
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}
