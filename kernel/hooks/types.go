// Package hooks 实现工具执行前的本地命令 Hook 与配置管理。
package hooks

// 数据。一条 PreToolUse 命令；Tools 为空时匹配全部工具。
type Hook struct {
	Name           string   `json:"name" jsonschema:"minLength=1"`
	Enabled        bool     `json:"enabled"`
	Tools          []string `json:"tools"`
	Command        string   `json:"command" jsonschema:"minLength=1"`
	Args           []string `json:"args"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

// 数据。一个来源的配置和读取版本。
type Source struct {
	Hooks []Hook `json:"hooks"`
	Hash  string `json:"hash"`
	Error string `json:"error,omitempty"`
}

// 数据。全局和当前项目的 Hook 设置投影。
type View struct {
	Global    Source `json:"global"`
	Project   Source `json:"project"`
	Workspace string `json:"workspace"`
	Trusted   bool   `json:"trusted"`
	LastError string `json:"lastError"`
}

// 数据。保存指定来源；Hash 防止覆盖读取后被修改的版本。
type SaveInput struct {
	Scope     string `json:"scope" jsonschema:"enum=global,enum=project"`
	Workspace string `json:"workspace"`
	Hash      string `json:"hash"`
	Hooks     []Hook `json:"hooks"`
}

// 数据。确认当前项目文件的内容版本，不修改配置。
type TrustInput struct {
	Workspace string `json:"workspace" jsonschema:"minLength=1"`
	Hash      string `json:"hash" jsonschema:"minLength=1"`
}
