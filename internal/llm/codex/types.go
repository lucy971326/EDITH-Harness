package codex

// 数据。ChatGPT 订阅登录状态；授权网址仅在等待登录时公开。
type View struct {
	Authenticated bool   `json:"authenticated"`
	State         string `json:"state"`
	URL           string `json:"url,omitempty"`
	Message       string `json:"message,omitempty"`
}
