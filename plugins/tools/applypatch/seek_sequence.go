package applypatch

import (
	"strings"
	"unicode"
)

func seekSequence(lines, pattern []string, start int, endOfFile bool) (int, bool) {
	if len(pattern) == 0 {
		return start, true
	}
	if len(pattern) > len(lines) {
		return 0, false
	}

	searchStart := start
	if endOfFile {
		endStart := len(lines) - len(pattern)
		if endStart > searchStart {
			searchStart = endStart
		}
	}
	if searchStart > len(lines)-len(pattern) {
		return 0, false
	}

	passes := []func(string) string{
		func(value string) string { return value },
		func(value string) string { return strings.TrimRightFunc(value, unicode.IsSpace) },
		strings.TrimSpace,
		normalizePunctuation,
	}
	for _, normalize := range passes {
		for index := searchStart; index <= len(lines)-len(pattern); index++ {
			matched := true
			for offset, expected := range pattern {
				if normalize(lines[index+offset]) != normalize(expected) {
					matched = false
					break
				}
			}
			if matched {
				return index, true
			}
		}
	}
	return 0, false
}

func normalizePunctuation(value string) string {
	return strings.Map(func(char rune) rune {
		switch char {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			return '-'
		case '\u2018', '\u2019', '\u201a', '\u201b':
			return '\''
		case '\u201c', '\u201d', '\u201e', '\u201f':
			return '"'
		case '\u00a0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200a', '\u202f', '\u205f', '\u3000':
			return ' '
		default:
			return char
		}
	}, strings.TrimSpace(value))
}
