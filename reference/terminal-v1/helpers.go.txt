package terminal

import (
	"fmt"
	"strconv"
	"strings"

	"harness/session"
)

func chooseTools(text string, names []string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	seen := make(map[int]bool)
	selected := make([]string, 0)
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		index, ok := parseChoice(part, len(names))
		if !ok {
			for i, name := range names {
				if name == part {
					index = i
					ok = true
					break
				}
			}
		}
		if !ok {
			return nil, fmt.Errorf("工具 %q 不存在", part)
		}
		if seen[index] {
			continue
		}
		seen[index] = true
		selected = append(selected, names[index])
	}
	return selected, nil
}

func parseChoice(text string, count int) (int, bool) {
	number, err := strconv.Atoi(text)
	if err != nil || number < 1 || number > count {
		return 0, false
	}
	return number - 1, true
}

func visibleEvents(events []session.Event) []session.Event {
	replaced := make(map[int]bool)
	for _, event := range events {
		for _, seq := range event.Replaces {
			replaced[seq] = true
		}
	}
	visible := make([]session.Event, 0, len(events))
	for _, event := range events {
		if !replaced[event.Seq] {
			visible = append(visible, event)
		}
	}
	return visible
}

func preview(text string) string {
	clean := safeText(text)
	if clean == "" {
		return ""
	}
	const limit = 240
	runes := []rune(clean)
	if len(runes) > limit {
		clean = string(runes[:limit]) + "…"
	}
	return clean
}

func safeText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 0x20 {
			return r
		}
		return '�'
	}, text)
}
