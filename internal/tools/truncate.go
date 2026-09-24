package tools

import (
	"unicode/utf8"
)

const (
	maxOutputBytes = 50 * 1024
	maxOutputLines = 2000
)

const truncatedNotice = "\n\n[output truncated: showing at most 2000 lines or 50 KiB]"

// TruncateHead 截断文本开头，供 read 保留文件开头。
func TruncateHead(text string) string {
	limited := firstLines(text)
	limited = firstBytes(limited)
	if limited == text {
		return text
	}
	return limited + truncatedNotice
}

// TruncateTail 截断文本结尾，供 bash 保留最新输出。
func TruncateTail(text string) string {
	limited := lastLines(text)
	limited = lastBytes(limited)
	if limited == text {
		return text
	}
	return truncatedNotice + "\n" + limited
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

func lastLines(text string) string {
	lines := 0
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] != '\n' {
			continue
		}
		lines++
		if lines == maxOutputLines && i+1 < len(text) {
			return text[i+1:]
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

func lastBytes(text string) string {
	if len(text) <= maxOutputBytes {
		return text
	}
	start := len(text) - maxOutputBytes
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return text[start:]
}
