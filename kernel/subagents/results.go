package subagents

import (
	"fmt"
	"strings"

	"harness/kernel/session"
)

// readResults 只读逐轮记录指定的最终助手消息，不重新猜测结果或复制到业务存储。
func (s *Subagents) readResults(task Task) ([]TurnResult, error) {
	results := make([]TurnResult, 0)
	var entries map[string]session.Entry
	for _, turn := range task.Turns {
		if turn.ResultEntryID == "" {
			continue
		}
		if entries == nil {
			child, err := s.sessions.Get(task.ChildSessionID)
			if err != nil {
				return results, fmt.Errorf("subagents: read task %s results: %w", task.ID, err)
			}
			entries = make(map[string]session.Entry)
			for _, entry := range child.Entries() {
				entries[entry.ID] = entry
			}
		}
		entry, ok := entries[turn.ResultEntryID]
		if !ok || entry.Message.RunID != turn.RunID || entry.Message.Role != session.RoleAssistant {
			return results, fmt.Errorf("subagents: task %s turn %d result entry missing or mismatched", task.ID, turn.Turn)
		}
		var text strings.Builder
		for _, block := range entry.Message.Blocks {
			if block.Kind == "tool-call" || block.Tool != nil {
				return results, fmt.Errorf("subagents: task %s turn %d result contains tool calls", task.ID, turn.Turn)
			}
			if block.Kind == "text" {
				text.WriteString(block.Text)
			}
		}
		results = append(results, TurnResult{Turn: turn.Turn, RunID: turn.RunID, EntryID: entry.ID, Text: text.String()})
	}
	return results, nil
}
