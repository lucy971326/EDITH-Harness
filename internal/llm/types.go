package llm

import (
	"encoding/json"
	"harness/internal/session"
	"harness/internal/tools"
)

// 数据。当前本机配置可供会话选择的一种模型。
type ModelChoice struct {
	ID               string   `json:"id"`
	Provider         string   `json:"provider"`
	ContextWindow    int      `json:"contextWindow"`
	Vision           bool     `json:"vision"`
	ReasoningEfforts []string `json:"reasoningEfforts"`
}

// 数据。设置页可见的供应商信息，绝不包含 API 密钥。
type ProviderSettings struct {
	ID        string `json:"id"`
	Protocol  string `json:"protocol"`
	BaseURL   string `json:"baseURL"`
	HasAPIKey bool   `json:"hasAPIKey"`
}

// 数据。图形化思考档位；Mode 为 off、enabled 或 adaptive。
type ReasoningSettings struct {
	Name         string `json:"name"`
	Mode         string `json:"mode"`
	Effort       string `json:"effort"`
	BudgetTokens int    `json:"budgetTokens"`
}

// 数据。设置页可编辑的单个模型。
type ModelSettings struct {
	Protocol      string              `json:"protocol,omitempty"`
	Key           string              `json:"key"`
	Provider      string              `json:"provider"`
	ID            string              `json:"id"`
	ContextWindow int                 `json:"contextWindow"`
	MaxOutput     int                 `json:"maxOutput,omitempty"`
	Manual        bool                `json:"manual,omitempty"`
	Vision        bool                `json:"vision"`
	Reasoning     []ReasoningSettings `json:"reasoning"`
}

// 数据。配置读写后的真实目录与文件版本。
type SettingsView struct {
	ProviderPresets  []ProviderPreset   `json:"providerPresets"`
	Providers        []ProviderSettings `json:"providers"`
	Models           []ModelSettings    `json:"models"`
	Presets          []ModelSettings    `json:"presets"`
	ProviderRevision string             `json:"providerRevision"`
	ModelRevision    string             `json:"modelRevision"`
}

// 数据。供应商保存输入；空 APIKey 保留现有密钥，ClearAPIKey 显式清除。
type SaveProviderInput struct {
	ID          string `json:"id"`
	Protocol    string `json:"protocol"`
	BaseURL     string `json:"baseURL"`
	APIKey      string `json:"apiKey"`
	ClearAPIKey bool   `json:"clearAPIKey"`
	Revision    string `json:"revision"`
}

// 数据。模型保存输入；Key 非空时只允许更新同一模型的能力。
type SaveModelInput struct {
	Model    ModelSettings `json:"model"`
	Revision string        `json:"revision"`
}

// 数据。一次模型调用使用的模型和思考档位。
type RunConfig struct {
	Model           string
	ReasoningEffort string
}

// 数据。一次模型调用的提示词、历史和工具定义。
type Input struct {
	System     string
	History    []session.Message
	Tools      []tools.Definition
	ToolChoice string
}

// 数据。模型流中的一项增量；工具调用只在参数完整后发出。
type StreamChunk struct {
	Type         string
	Text         string
	ToolCallID   string
	ToolName     string
	ToolInput    string
	Usage        Usage
	FinishReason string
	Error        error
	Continuation *session.ModelContinuation
}

const (
	ChunkText         = "text"
	ChunkReasoning    = "reasoning"
	ChunkToolCall     = "tool-call"
	ChunkFinish       = "finish"
	ChunkError        = "error"
	ChunkContinuation = "continuation"
	FinishStop        = "stop"
	FinishLength      = "length"
)

// 数据。一次请求的 token 用量，InputTokens 不含命中的缓存，CacheReadTokens 单独统计。
type Usage struct {
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
}

// 模型协议转换前的消息，不依赖 SDK。
type modelMessage struct {
	Role    string
	Content []part
}
type part struct {
	Type         string
	Text         string
	URL          string
	MediaType    string
	ToolCallID   string
	ToolName     string
	ToolInput    json.RawMessage
	ToolOutput   string
	IsError      bool
	Continuation *session.ModelContinuation
}
type toolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

const (
	RoleSystem     = "system"
	RoleUser       = "user"
	RoleAssistant  = "assistant"
	RoleTool       = "tool"
	PartText       = "text"
	PartReasoning  = "reasoning"
	PartImage      = "image"
	PartToolCall   = "tool-call"
	PartToolResult = "tool-result"
)

// 数据。统一结束原因。
type FinishReason = string

// 数据。生成的供应商预设，无凭据；思考格式只供后端参数转换。
type ProviderPreset struct {
	ID             string `json:"id,omitempty"`
	Name           string `json:"name"`
	Protocol       string `json:"protocol"`
	BaseURL        string `json:"baseURL"`
	ThinkingFormat string `json:"thinkingFormat,omitempty"`
}
