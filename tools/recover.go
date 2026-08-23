package tools

import "harness/session"

// RecoverDangling 给恢复流程用：给一笔悬空的工具调用补上终局。
// 工具的账归 tools 管（一家一种账），loop 恢复时也不绕过这个规矩。
//
// 两种悬空、两种补法：
//   - 有 start 无 result：副作用可能已发生——补 status:"unknown"，正文写给模型的决策指引；
//   - 无 start 无 result：本体没开跑——补 status:"skipped"，模型可以放心重试。
func RecoverDangling(book *session.Session, callID string, status string, note string) error {
	_, err := book.RecordToolResult(session.ToolResultData{
		CallID: callID,
		Output: note,
		Status: status,
	})
	return err
}
