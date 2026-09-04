// Package runview 提供 Runner 驱动产品共用的 Web 运行视图。
package runview

import (
	"harness/kernel/runner"
	"harness/kernel/session"
)

// 数据。Snapshot 是浏览器恢复一场 Session 运行视图所需的耐久事实与活跃 Run。
type Snapshot struct {
	Entries []session.Entry   `json:"entries"`
	Runs    []runner.RunState `json:"runs"`
}

// 数据。Config 指定一个 RunView 的浏览器数据来源与临时显示行为。
type Config struct {
	ID          string
	SnapshotURL string
	EventsURL   string
	AutoScroll  bool
}
