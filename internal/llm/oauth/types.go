package oauth

// 数据。设置页可见的账号登录状态；不包含令牌。
type View struct {
	Authenticated bool   `json:"authenticated"`
	State         string `json:"state"`
	URL           string `json:"url,omitempty"`
	UserCode      string `json:"userCode,omitempty"`
	Message       string `json:"message,omitempty"`
}

// 数据。本次请求的内存凭据，不进入 RPC 或模型配置。
type Authorization struct {
	Token     string
	AccountID string
}
