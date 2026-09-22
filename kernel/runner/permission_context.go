package runner

import "harness/kernel/session"

// permissionHistory 只重建模型输入，不向对话账本添加设置或审批状态。
// 每轮记录保留当时的说明；相邻相同说明省略，旧前缀不会因切模式被改写。
func permissionHistory(history []session.Message, records []runRecord) []session.Message {
	instructions := make(map[string]string, len(records))
	for _, record := range records {
		instructions[record.RunID] = record.PermissionInstructions
	}
	result := make([]session.Message, 0, len(history))
	seen := make(map[string]bool)
	previous := ""
	for _, message := range history {
		if !seen[message.RunID] {
			seen[message.RunID] = true
			text := instructions[message.RunID]
			if text != "" && text != previous {
				result = append(result, session.Message{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: text}}})
				previous = text
			}
		}
		result = append(result, message)
	}
	return result
}
