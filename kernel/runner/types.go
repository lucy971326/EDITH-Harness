// Package runner 定义一轮对话运行外壳。
package runner

import "harness/kernel/loops"

// 数据。Runner 发布给界面的一条本轮事件。
type RunEvent struct {
	SessionID string
	Event     loops.Event
}
