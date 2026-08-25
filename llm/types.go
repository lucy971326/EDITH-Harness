// Package llm 是插座排：消费方只认识这里，从不 import 任何模型服务商。
// 适配器（真正去调某家 API 的翻译官）全是将来的插件；真货 deepseek/openai、假货测试适配器都往这插。
package llm

import (
	"context"

	"harness/chat"
)

// Adapter 是一个模型服务商的适配器：把统一的请求翻译成它家的 API 调用，
// 把它家的响应翻译回统一的回复。适配器内部随便抛错，出口会被包成统一错误。
type Adapter interface {
	// Name 服务商名，插座排按它路由："deepseek" / "openai"。
	Name() string
	// Stream 发一次请求，模型吐字时通过 onDelta 一小截一小截递出；
	// 吐完了返回整份回复（聚合好的全文 + 工具调用 + 用量）。
	// 不支持流式的服务商就不吐 delta，直接回——适配器只需实现这一个方法。
	Stream(ctx context.Context, req Request, onDelta func(chat.Delta)) (Reply, error)
}

// ModelInfo 是服务商声明的一个可选模型及其能力。
type ModelInfo struct {
	ID                      string   // 发请求时使用的模型标识
	Name                    string   // 界面展示名称；空时退回 ID
	ThinkingLevels          []string // 这个模型支持的思考档位
	SupportsProviderDefault bool     // 是否允许把思考档位交给服务商自行决定
}

// ProviderInfo 是界面可展示的一家模型服务商及其模型目录。
type ProviderInfo struct {
	Name        string      // 插座上的服务商名
	DisplayName string      // 界面展示名称；空时退回 Name
	Models      []ModelInfo // 已适配且可让用户选择的模型
}

// ProviderCatalog 是适配器提供给界面的模型目录。
// 目录只描述这个适配器实际支持的模型和能力，不代表全网服务商。
type ProviderCatalog interface {
	ProviderInfo() ProviderInfo
}

// Selection 是一段会话当前选中的模型组合。
type Selection struct {
	Provider string // 服务商插座名
	Model    string // 服务商内的模型标识
	Thinking string // 该模型支持的思考档位；空值表示使用服务商默认
}

// Request 是一次模型请求的全部输入。
type Request struct {
	Provider    string            // 服务商名，如 "deepseek"；必须明确，不猜默认插座
	Model       string            // 服务商内的型号名，如 "deepseek-chat"
	Thinking    string            // 思考强度：由服务商插件解释，空值表示使用服务商默认
	Messages    []chat.Message    // 完整对话历史（system 消息也在里面，各服务商自己认领）
	Tools       []chat.ToolSchema // 可用的工具说明书，空 = 这次不许调工具
	MaxTokens   int               // 回复上限，0 = 用服务商默认
	Temperature float64           // 温度，0 = 用服务商默认
}

// Reply 是模型一次回复的完整聚合。
type Reply struct {
	Text       string          // 说了什么字
	Thinking   string          // 模型的隐藏思考；要入账，续聊时再交还给服务商
	Calls      []chat.ToolCall // 要调哪些工具（可以边说边调），没调用就为空
	Usage      chat.Usage      // 这次花了多少 token
	StopReason string          // 为什么停："stop" 说完了 / "tool_calls" 要调工具 / "length" 到字数上限 / "cancelled" 被取消
}
