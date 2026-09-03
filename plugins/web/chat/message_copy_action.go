package chat

import (
	"context"
	"fmt"
	"strings"

	"harness/kernel/session"
)

// 活对象。copyMessageAction 复制耐久用户消息或助手卡片的正文。
type copyMessageAction struct{}

func (copyMessageAction) Definition() MessageActionDefinition {
	return MessageActionDefinition{
		ID:      "copy",
		Name:    "复制",
		Icon:    "⧉",
		Order:   10,
		Targets: []MessageCardType{MessageCardUser, MessageCardAssistant},
	}
}

func (copyMessageAction) Execute(_ context.Context, input MessageActionContext) (MessageActionResult, error) {
	var text string
	switch input.Target.CardType {
	case MessageCardUser:
		entry, ok := entryByID(input.Entries, input.Target.EntryID)
		if !ok || entry.Message.Role != session.RoleUser {
			return MessageActionResult{}, fmt.Errorf("chat: copy target user message not found")
		}
		text = textBlocks(entry.Message)
	case MessageCardAssistant:
		text = assistantSegmentText(input.Entries, input.Target.RunID, input.Target.BoundaryEntryID)
	default:
		return MessageActionResult{}, fmt.Errorf("chat: copy target has unknown card type %q", input.Target.CardType)
	}
	if text == "" {
		return MessageActionResult{}, fmt.Errorf("chat: copy target has no text")
	}
	return MessageActionResult{Text: text}, nil
}

func entryByID(entries []session.Entry, id string) (session.Entry, bool) {
	for _, entry := range entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return session.Entry{}, false
}

func assistantSegmentText(entries []session.Entry, runID, boundaryEntryID string) string {
	start := assistantSegmentStart(entries, runID, boundaryEntryID)
	if start < 0 {
		return ""
	}
	var lastText string
	for _, entry := range entries[start+1:] {
		if entry.Message.RunID != runID {
			continue
		}
		if entry.Message.Role == session.RoleUser {
			break
		}
		if entry.Message.Role == session.RoleAssistant {
			if text := textBlocks(entry.Message); text != "" {
				lastText = text
			}
		}
	}
	return lastText
}

func assistantSegmentStart(entries []session.Entry, runID, boundaryEntryID string) int {
	for index, entry := range entries {
		if entry.ID == boundaryEntryID && entry.Message.Role == session.RoleUser && entry.Message.RunID == runID {
			return index
		}
	}
	return -1
}

func textBlocks(message session.Message) string {
	var texts []string
	for _, block := range message.Blocks {
		if block.Kind == "text" {
			texts = append(texts, block.Text)
		}
	}
	return strings.Join(nonEmpty(texts), "\n\n")
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
