package llm

import (
	"context"

	"harness/kernel/session"
)

// Config 是可连接 Provider 的部署配置。Models 必须嵌在 Provider 下。
type Config struct {
	Providers map[string]ProviderConfig
}

// ProviderConfig 是 endpoint、认证和该 Provider 默认协议。
type ProviderConfig struct {
	Catalog   string
	API       string
	BaseURL   string
	APIKeyEnv string
	Models    map[string]ModelConfig
}

// ModelConfig 启用一个模型。ID 为空时，模型名就是远端 model ID。
type ModelConfig struct {
	ID  string
	API string
}

// Catalog 是离线处理后的模型事实。键是 models.dev Provider 名。
type Catalog struct {
	Providers map[string]CatalogProvider
}

type CatalogProvider struct {
	Models map[string]CatalogModel
}

// CatalogModel 是路由和 UI 需要的模型事实。Reasoning 的键是 Setup 可选档位，
// 值是该档位需要写入协议 body 的完整顶层字段。
type CatalogModel struct {
	Vision        bool
	ContextWindow int
	Tools         bool
	Reasoning     map[string]Patch
}

// Patch 是 Catalog 中可信的协议补充字段。它不是用户输入。
type Patch map[string]any

// Request 是调用者已经组装好的本轮输入。模型和思考档位只来自 Setup。
type Request struct {
	System   string
	Messages []session.Message
}

// Target 是本次调用已经解析完成的远端目标。
type Target struct {
	Key       string // Provider/Model，如 deepseek/deepseek-chat
	Provider  string
	Model     string
	RemoteID  string
	API       string
	BaseURL   string
	APIKeyEnv string
	Vision    bool
}

// Call 是 Service 交给协议 Adapter 的完整调用。
type Call struct {
	Target    Target
	System    string
	Messages  []session.Message
	Reasoning Patch
}

// Usage 仅保留远端报告的 token 计数；耗时由 Runner 以后记录。
type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// Chunk 是不同协议流统一后的片段。
type Chunk struct {
	Index int
	Kind  string // reasoning | text | tool-call
	Delta string
	Tool  *session.ToolCall
	Done  bool
	Usage *Usage
	Err   error
}

// Adapter 只负责一种已编译协议的请求与流转换。
type Adapter interface {
	Stream(context.Context, Call) (<-chan Chunk, error)
}
