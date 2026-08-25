package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"

	"harness/chat"
	"harness/llm"
)

// adapter 把 Harness 对话翻译给 DeepSeek 的 OpenAI 兼容接口。
type adapter struct {
	completions openai.ChatCompletionService // 显式密钥和地址创建的官方 SDK 服务
}

func newAdapter(apiKey string, baseURL string) *adapter {
	completions := openai.NewChatCompletionService(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	)
	return &adapter{completions: completions}
}

// Name 返回 llm 插座上的固定服务商名。
func (a *adapter) Name() string {
	return "deepseek"
}

// ProviderInfo 返回 DeepSeek 已适配模型及其思考档位。
func (a *adapter) ProviderInfo() llm.ProviderInfo {
	return llm.ProviderInfo{
		Name:        a.Name(),
		DisplayName: "DeepSeek",
		Models: []llm.ModelInfo{
			{
				ID:                      "deepseek-v4-flash",
				Name:                    "DeepSeek V4 Flash",
				ThinkingLevels:          []string{"off", "low", "high", "max"},
				SupportsProviderDefault: true,
			},
			{
				ID:                      "deepseek-v4-pro",
				Name:                    "DeepSeek V4 Pro",
				ThinkingLevels:          []string{"off", "high", "max"},
				SupportsProviderDefault: true,
			},
		},
	}
}

// Stream 用官方 SDK 流式请求 DeepSeek，输出可见文字并聚合思考、工具和用量。
func (a *adapter) Stream(ctx context.Context, req llm.Request, onDelta func(chat.Delta)) (llm.Reply, error) {
	err := validateThinking(req.Thinking)
	if err != nil {
		return llm.Reply{}, err
	}
	params, err := buildParams(req)
	if err != nil {
		return llm.Reply{}, err
	}
	stream := a.completions.NewStreaming(ctx, params)
	defer stream.Close()

	var reply llm.Reply
	calls := make(map[int64]*toolCall)
	for stream.Next() {
		chunk := stream.Current()
		if chunk.JSON.Usage.Valid() {
			reply.Usage = chat.Usage{
				InputTokens:  int(chunk.Usage.PromptTokens),
				OutputTokens: int(chunk.Usage.CompletionTokens),
			}
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta
			if delta.Content != "" {
				reply.Text += delta.Content
				if onDelta != nil {
					onDelta(chat.Delta{Text: delta.Content})
				}
			}
			thinking, err := extraString(delta.RawJSON(), delta.JSON.ExtraFields, "reasoning_content")
			if err != nil {
				return llm.Reply{}, err
			}
			reply.Thinking += thinking
			for _, fragment := range delta.ToolCalls {
				call := calls[fragment.Index]
				if call == nil {
					call = &toolCall{}
					calls[fragment.Index] = call
				}
				if fragment.ID != "" {
					call.id = fragment.ID
				}
				if fragment.Function.Name != "" {
					call.name = fragment.Function.Name
				}
				call.arguments.WriteString(fragment.Function.Arguments)
			}
			if choice.FinishReason != "" {
				reply.StopReason = choice.FinishReason
			}
		}
	}
	err = stream.Err()
	if err != nil {
		return llm.Reply{}, classifyError(err)
	}

	indices := make([]int64, 0, len(calls))
	for index := range calls {
		indices = append(indices, index)
	}
	sort.Slice(indices, func(left int, right int) bool { return indices[left] < indices[right] })
	for _, index := range indices {
		call := calls[index]
		reply.Calls = append(reply.Calls, chat.ToolCall{
			ID:       call.id,
			Name:     call.name,
			Argument: json.RawMessage(call.arguments.String()),
		})
	}
	if len(reply.Calls) > 0 && reply.StopReason == "" {
		reply.StopReason = "tool_calls"
	}
	return reply, nil
}

// toolCall 攒一条被拆开的工具调用。
type toolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

func validateThinking(thinking string) error {
	switch thinking {
	case "", "off", "low", "high", "max":
		return nil
	default:
		return llm.NewError("deepseek", llm.ErrBadRequest, "thinking 只支持 off、low、high 或 max")
	}
}

func buildParams(req llm.Request) (openai.ChatCompletionNewParams, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))
	for _, message := range req.Messages {
		converted, err := buildMessage(message)
		if err != nil {
			return openai.ChatCompletionNewParams{}, err
		}
		messages = append(messages, converted)
	}
	tools, err := buildTools(req.Tools)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(req.Model),
		Messages: messages,
		Tools:    tools,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
	}
	if req.MaxTokens > 0 {
		params.MaxTokens = openai.Int(int64(req.MaxTokens))
	}
	if req.Temperature != 0 {
		params.Temperature = openai.Float(req.Temperature)
	}
	if req.Thinking != "" {
		thinking := map[string]any{"type": "disabled"}
		extraFields := map[string]any{"thinking": thinking}
		if req.Thinking != "off" {
			thinking["type"] = "enabled"
			extraFields["reasoning_effort"] = req.Thinking
		}
		params.SetExtraFields(extraFields)
	}
	return params, nil
}

func buildMessage(message chat.Message) (openai.ChatCompletionMessageParamUnion, error) {
	if len(message.Media) > 0 {
		return openai.ChatCompletionMessageParamUnion{}, llm.NewError("deepseek", llm.ErrBadRequest, "DeepSeek 适配器暂不支持媒体消息")
	}
	switch message.Role {
	case "system":
		return openai.SystemMessage(message.Text), nil
	case "user":
		return openai.UserMessage(message.Text), nil
	case "tool":
		if message.Result == nil {
			return openai.ChatCompletionMessageParamUnion{}, llm.NewError("deepseek", llm.ErrBadRequest, "工具消息缺少结果")
		}
		return openai.ToolMessage(message.Result.Output, message.Result.CallID), nil
	case "assistant":
		assistant := openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(message.Text)},
		}
		for _, call := range message.Calls {
			assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: call.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      call.Name,
						Arguments: string(call.Argument),
					},
				},
			})
		}
		if message.Thinking != "" {
			assistant.SetExtraFields(map[string]any{"reasoning_content": message.Thinking})
		}
		return openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant}, nil
	default:
		return openai.ChatCompletionMessageParamUnion{}, llm.NewError("deepseek", llm.ErrBadRequest, fmt.Sprintf("不认识的消息角色 %q", message.Role))
	}
}

func buildTools(schemas []chat.ToolSchema) ([]openai.ChatCompletionToolUnionParam, error) {
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(schemas))
	for _, schema := range schemas {
		parameters := make(openai.FunctionParameters)
		if len(schema.Parameters) > 0 {
			err := json.Unmarshal(schema.Parameters, &parameters)
			if err != nil {
				return nil, llm.NewError("deepseek", llm.ErrBadRequest, fmt.Sprintf("工具 %s 的参数说明不是 JSON：%v", schema.Name, err))
			}
		}
		tools = append(tools, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        schema.Name,
			Description: openai.String(schema.Description),
			Parameters:  parameters,
		}))
	}
	return tools, nil
}

func extraString(rawJSON string, fields map[string]respjson.Field, name string) (string, error) {
	field, exists := fields[name]
	if exists && field.Valid() {
		var value string
		err := json.Unmarshal([]byte(field.Raw()), &value)
		if err != nil {
			return "", llm.NewError("deepseek", llm.ErrUnknown, fmt.Sprintf("DeepSeek 的 %s 不是字符串：%v", name, err))
		}
		return value, nil
	}
	var extended map[string]json.RawMessage
	err := json.Unmarshal([]byte(rawJSON), &extended)
	if err != nil {
		return "", llm.NewError("deepseek", llm.ErrUnknown, fmt.Sprintf("DeepSeek 的流式片段不是 JSON：%v", err))
	}
	valueJSON, exists := extended[name]
	if !exists || string(valueJSON) == "null" {
		return "", nil
	}
	var value string
	err = json.Unmarshal(valueJSON, &value)
	if err != nil {
		return "", llm.NewError("deepseek", llm.ErrUnknown, fmt.Sprintf("DeepSeek 的 %s 不是字符串：%v", name, err))
	}
	return value, nil
}

func classifyError(err error) error {
	if errors.Is(err, context.Canceled) {
		return llm.NewError("deepseek", llm.ErrCancelled, err.Error())
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden:
			return llm.NewError("deepseek", llm.ErrAuth, apiErr.Message)
		case apiErr.StatusCode == http.StatusTooManyRequests:
			return llm.NewError("deepseek", llm.ErrRateLimit, apiErr.Message)
		case apiErr.StatusCode >= 400 && apiErr.StatusCode < 500:
			return llm.NewError("deepseek", llm.ErrBadRequest, apiErr.Message)
		case apiErr.StatusCode >= 500:
			return llm.NewError("deepseek", llm.ErrServer, apiErr.Message)
		}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return llm.NewError("deepseek", llm.ErrNetwork, err.Error())
	}
	return llm.NewError("deepseek", llm.ErrUnknown, err.Error())
}
