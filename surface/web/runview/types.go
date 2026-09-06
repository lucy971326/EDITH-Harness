// Package runview 提供 Runner 驱动产品共用的 Web 运行视图。
package runview

// 数据。Config 指定一个 RunView 的浏览器数据来源与临时显示行为。
type Config struct {
	ID          string
	SnapshotURL string
	EventsURL   string
	AutoScroll  bool
}
