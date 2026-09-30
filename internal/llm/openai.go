package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
)

func (r streamRequest) openAIClient() openai.Client {
	options := []option.RequestOption{option.WithAPIKey(r.token), option.WithBaseURL(r.baseURL), option.WithMaxRetries(0), option.WithOrganization(""), option.WithProject("")}
	if r.definition.Provider == "cloudflare-ai-gateway" {
		options = append(options, option.WithHeaderDel("Authorization"), option.WithHeader("cf-aig-authorization", "Bearer "+r.token))
	}
	for key, value := range r.headers {
		options = append(options, option.WithHeader(key, value))
	}
	return openai.NewClient(options...)
}

func (r streamRequest) chatBody() map[string]any {
	messages := []any{}
	if r.system != "" {
		messages = append(messages, map[string]any{"role": "system", "content": r.system})
	}
	for _, message := range r.messages {
		content := []any{}
		calls := []any{}
		var thinking strings.Builder
		var text strings.Builder
		var details json.RawMessage
		hasImage := false
		for _, part := range message.Content {
			switch part.Type {
			case PartText:
				text.WriteString(part.Text)
				content = append(content, map[string]any{"type": "text", "text": part.Text})
			case PartReasoning:
				thinking.WriteString(part.Text)
				if r.accepts(part.Continuation) {
					details = part.Continuation.Data
				}
			case PartImage:
				hasImage = true
				content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": part.URL}})
			case PartToolCall:
				call := map[string]any{"id": part.ToolCallID, "type": "function", "function": map[string]any{"name": part.ToolName, "arguments": string(part.ToolInput)}}
				if r.accepts(part.Continuation) {
					call["extra_content"] = part.Continuation.Data
				}
				calls = append(calls, call)
			case PartToolResult:
				messages = append(messages, map[string]any{"role": "tool", "tool_call_id": part.ToolCallID, "content": part.ToolOutput})
			}
		}
		if message.Role == RoleTool {
			continue
		}
		entry := map[string]any{"role": message.Role, "content": content}
		if !hasImage {
			entry["content"] = text.String()
		}
		if len(details) > 0 {
			entry["reasoning_details"] = details
		}
		if len(calls) > 0 {
			entry["tool_calls"] = calls
		}
		// 只有声明 interleaved reasoning 的供应商需要回传明文思考。
		if thinking.Len() > 0 && (catalog.Providers[r.definition.Provider].ThinkingFormat == "deepseek" || catalog.Providers[r.definition.Provider].ThinkingFormat == "qwen") {
			entry["reasoning_content"] = thinking.String()
		}
		messages = append(messages, entry)
	}
	body := map[string]any{"model": r.definition.ID, "messages": messages, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	if r.definition.MaxOutput > 0 {
		field := "max_tokens"
		if r.definition.Provider == "openai" {
			field = "max_completion_tokens"
		}
		body[field] = r.definition.MaxOutput
	}
	if len(r.tools) > 0 {
		tools := []any{}
		for _, tool := range r.tools {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema}})
		}
		body["tools"] = tools
		if r.toolChoice != "" {
			body["tool_choice"] = r.toolChoice
		}
	}
	for key, value := range r.options {
		body[key] = value
	}
	return body
}

type pendingCall struct {
	id, name, arguments string
	extra               json.RawMessage
}

func (s streamSink) chat() error {
	client := s.request.openAIClient()
	var response *http.Response
	stream := client.Chat.Completions.NewStreaming(s.ctx, param.Override[openai.ChatCompletionNewParams](s.request.chatBody()), option.WithResponseInto(&response))
	defer stream.Close()
	calls := map[int]*pendingCall{}
	var usage Usage
	finish := ""
	details := []map[string]any{}
	for stream.Next() {
		var event struct {
			Choices []struct {
				Index int `json:"index"`
				Delta struct {
					Content       string           `json:"content"`
					Reasoning     string           `json:"reasoning_content"`
					ReasoningText string           `json:"reasoning"`
					Details       []map[string]any `json:"reasoning_details"`
					Calls         []struct {
						Extra    json.RawMessage `json:"extra_content"`
						Index    int             `json:"index"`
						ID       string          `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				Input   int `json:"prompt_tokens"`
				Output  int `json:"completion_tokens"`
				Details struct {
					Cached int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
				CacheHit int `json:"prompt_cache_hit_tokens"`
				Cached   int `json:"cached_tokens"`
			} `json:"usage"`
		}
		err := json.Unmarshal([]byte(stream.Current().RawJSON()), &event)
		if err != nil {
			return err
		}
		if event.Usage.Input > 0 {
			cached := max(event.Usage.Details.Cached, event.Usage.CacheHit, event.Usage.Cached)
			usage = Usage{InputTokens: max(0, event.Usage.Input-cached), OutputTokens: event.Usage.Output, CacheReadTokens: cached}
		}
		for _, choice := range event.Choices {
			if choice.Index != 0 {
				continue
			}
			if choice.Delta.Content != "" && !s.send(StreamChunk{Type: ChunkText, Text: choice.Delta.Content}) {
				return s.ctx.Err()
			}
			reasoning := choice.Delta.Reasoning
			if reasoning == "" {
				reasoning = choice.Delta.ReasoningText
			}
			if reasoning != "" && !s.send(StreamChunk{Type: ChunkReasoning, Text: reasoning}) {
				return s.ctx.Err()
			}
			for _, detail := range choice.Delta.Details {
				kind, _ := detail["type"].(string)
				if kind != "reasoning.text" && kind != "reasoning.summary" && kind != "reasoning.encrypted" {
					continue
				}
				last := len(details) - 1
				if last >= 0 && kind != "reasoning.encrypted" && details[last]["type"] == kind && details[last]["index"] == detail["index"] {
					field := "text"
					if kind == "reasoning.summary" {
						field = "summary"
					}
					previous, _ := details[last][field].(string)
					delta, _ := detail[field].(string)
					details[last][field] = previous + delta
					for key, value := range detail {
						if old, ok := details[last][key]; !ok || old == nil {
							details[last][key] = value
						}
					}
				} else {
					details = append(details, detail)
				}
			}
			for _, delta := range choice.Delta.Calls {
				call := calls[delta.Index]
				if call == nil {
					call = &pendingCall{}
					calls[delta.Index] = call
				}
				if delta.ID != "" {
					call.id = delta.ID
				}
				if delta.Function.Name != "" {
					call.name += delta.Function.Name
				}
				call.arguments += delta.Function.Arguments
				if len(delta.Extra) > 0 {
					call.extra = delta.Extra
				}
			}
			if choice.Finish != "" {
				finish = choice.Finish
			}
		}
	}
	if err := stream.Err(); err != nil {
		return openAIStreamError(err, response)
	}
	if finish == "" {
		return fmt.Errorf("llm: stream ended without finish reason")
	}
	if len(details) > 0 {
		raw, err := json.Marshal(details)
		if err != nil {
			return err
		}
		s.continuation(raw)
	}
	if finish == "tool_calls" || finish == "function_call" || finish == "stop" {
		indices := make([]int, 0, len(calls))
		for index := range calls {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			call := calls[index]
			err := s.call(call.id, call.name, call.arguments, call.extra)
			if err != nil {
				return err
			}
		}
	}
	if finish != "stop" && finish != "tool_calls" && finish != "function_call" && finish != "length" {
		return fmt.Errorf("llm: model stopped: %s", finish)
	}
	s.send(StreamChunk{Type: ChunkFinish, Usage: usage, FinishReason: finish})
	return nil
}

// xAI 等兼容服务返回字符串 error；官方 SDK 仅识别对象，会丢失真正的失败原因。
func openAIStreamError(err error, response *http.Response) error {
	if response == nil || response.StatusCode < 400 || response.Body == nil {
		return err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if readErr != nil {
		return err
	}
	var failure struct {
		Error string `json:"error"`
	}
	decodeErr := json.Unmarshal(body, &failure)
	if decodeErr == nil && failure.Error != "" {
		if response.StatusCode == 400 || response.StatusCode == 413 {
			if contextErr := contextError("", failure.Error); contextErr != nil {
				return contextErr
			}
		}
		return fmt.Errorf("llm: HTTP %d: %s", response.StatusCode, failure.Error)
	}
	return err
}
