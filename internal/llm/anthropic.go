package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

func (r streamRequest) anthropicBody() map[string]any {
	messages := []map[string]any{}
	system := r.system
	for _, message := range r.messages {
		content := []any{}
		role := message.Role
		if role == RoleTool {
			role = RoleUser
		}
		for _, part := range message.Content {
			if message.Role == RoleSystem {
				system += "\n" + part.Text
				continue
			}
			switch part.Type {
			case PartText:
				if part.Text != "" {
					content = append(content, map[string]any{"type": "text", "text": part.Text})
				}
			case PartImage:
				_, data, _ := strings.Cut(part.URL, ",")
				content = append(content, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": part.MediaType, "data": data}})
			case PartReasoning:
				if r.accepts(part.Continuation) {
					content = append(content, part.Continuation.Data)
				}
			case PartToolCall:
				content = append(content, map[string]any{"type": "tool_use", "id": part.ToolCallID, "name": part.ToolName, "input": part.ToolInput})
			case PartToolResult:
				content = append(content, map[string]any{"type": "tool_result", "tool_use_id": part.ToolCallID, "content": part.ToolOutput, "is_error": part.IsError})
			}
		}
		if len(content) == 0 {
			continue
		}
		// 合并连续同角色消息，保持一批并行 tool_result 在同一个 user 消息内。
		if len(messages) > 0 && messages[len(messages)-1]["role"] == role {
			last := messages[len(messages)-1]
			last["content"] = append(last["content"].([]any), content...)
		} else {
			messages = append(messages, map[string]any{"role": role, "content": content})
		}
	}
	maximum := r.definition.MaxOutput
	if maximum <= 0 {
		maximum = min(16384, r.definition.ContextWindow)
	}
	body := map[string]any{"model": r.definition.ID, "messages": messages, "max_tokens": maximum, "stream": true}
	if system != "" {
		body["system"] = system
	}
	tools := []any{}
	for _, tool := range r.tools {
		tools = append(tools, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema})
	}
	if len(tools) > 0 {
		body["tools"] = tools
		if r.toolChoice != "" {
			kind := r.toolChoice
			if kind == "required" {
				kind = "any"
			}
			body["tool_choice"] = map[string]any{"type": kind}
		}
	}
	if thinking, ok := r.options["thinking"].(map[string]any); ok {
		value := map[string]any{"type": thinking["type"]}
		if budget, ok := thinking["budgetTokens"]; ok {
			value["budget_tokens"] = budget
		}
		body["thinking"] = value
	}
	if effort, ok := r.options["effort"]; ok && effort != "" {
		body["output_config"] = map[string]any{"effort": effort}
	}
	return body
}

func (s streamSink) anthropic() error {
	r := s.request
	options := []option.RequestOption{option.WithAPIKey(r.token), option.WithAuthToken(""), option.WithBaseURL(r.baseURL), option.WithMaxRetries(0)}
	switch r.definition.Provider {
	case "github-copilot", "vercel-ai-gateway":
		options = append(options, option.WithHeaderDel("x-api-key"), option.WithAuthToken(r.token))
	case "cloudflare-ai-gateway":
		options = append(options, option.WithHeaderDel("x-api-key"), option.WithHeaderDel("Authorization"), option.WithHeader("cf-aig-authorization", "Bearer "+r.token))
	}
	for key, value := range r.headers {
		options = append(options, option.WithHeader(key, value))
	}
	client := anthropic.NewClient(options...)
	stream := client.Messages.NewStreaming(s.ctx, param.Override[anthropic.MessageNewParams](r.anthropicBody()))
	defer stream.Close()
	blocks := map[int]map[string]any{}
	inputs := map[int]string{}
	var usage Usage
	finish := ""
	stopped := false
	for stream.Next() {
		var event struct {
			Type  string         `json:"type"`
			Index int            `json:"index"`
			Block map[string]any `json:"content_block"`
			Delta struct {
				Type      string `json:"type"`
				Text      string `json:"text"`
				Thinking  string `json:"thinking"`
				Signature string `json:"signature"`
				Input     string `json:"partial_json"`
				Stop      string `json:"stop_reason"`
			} `json:"delta"`
			Message struct {
				Usage struct {
					Input   int `json:"input_tokens"`
					Cached  int `json:"cache_read_input_tokens"`
					Created int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				Output int `json:"output_tokens"`
			} `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		err := json.Unmarshal([]byte(stream.Current().RawJSON()), &event)
		if err != nil {
			return err
		}
		switch event.Type {
		case "message_start":
			u := event.Message.Usage
			usage.InputTokens = u.Input + u.Created
			usage.CacheReadTokens = u.Cached
		case "content_block_start":
			blocks[event.Index] = event.Block
			if value, ok := event.Block["text"].(string); ok && value != "" {
				s.send(StreamChunk{Type: ChunkText, Text: value})
			}
			if value, ok := event.Block["thinking"].(string); ok && value != "" {
				s.send(StreamChunk{Type: ChunkReasoning, Text: value})
			}
		case "content_block_delta":
			block := blocks[event.Index]
			if block == nil {
				return fmt.Errorf("llm: delta has no content block")
			}
			switch event.Delta.Type {
			case "text_delta":
				if !s.send(StreamChunk{Type: ChunkText, Text: event.Delta.Text}) {
					return s.ctx.Err()
				}
			case "thinking_delta":
				old, _ := block["thinking"].(string)
				block["thinking"] = old + event.Delta.Thinking
				if !s.send(StreamChunk{Type: ChunkReasoning, Text: event.Delta.Thinking}) {
					return s.ctx.Err()
				}
			case "signature_delta":
				old, _ := block["signature"].(string)
				block["signature"] = old + event.Delta.Signature
			case "input_json_delta":
				inputs[event.Index] += event.Delta.Input
			}
		case "content_block_stop":
			block := blocks[event.Index]
			if block == nil {
				return fmt.Errorf("llm: stop has no content block")
			}
			switch block["type"] {
			case "tool_use":
				id, _ := block["id"].(string)
				name, _ := block["name"].(string)
				arguments := inputs[event.Index]
				if arguments == "" {
					raw, err := json.Marshal(block["input"])
					if err != nil {
						return err
					}
					arguments = string(raw)
				}
				err = s.call(id, name, arguments, nil)
				if err != nil {
					return err
				}
			case "thinking", "redacted_thinking":
				raw, err := json.Marshal(block)
				if err != nil {
					return err
				}
				s.continuation(raw)
			}
			delete(blocks, event.Index)
			delete(inputs, event.Index)
		case "message_delta":
			finish = event.Delta.Stop
			usage.OutputTokens = event.Usage.Output
		case "message_stop":
			stopped = true
		case "error":
			if err := contextError("", event.Error.Message); err != nil {
				return err
			}
			if event.Error.Message != "" {
				return fmt.Errorf("llm: Anthropic stream failed: %s", event.Error.Message)
			}
			return fmt.Errorf("llm: Anthropic stream failed")
		}
	}
	if err := stream.Err(); err != nil {
		return err
	}
	if !stopped || finish == "" || len(blocks) > 0 {
		return fmt.Errorf("llm: Anthropic stream interrupted before completion")
	}
	switch finish {
	case "end_turn", "stop_sequence":
		finish = "stop"
	case "max_tokens":
		finish = "length"
	case "tool_use":
		finish = "tool_calls"
	default:
		return fmt.Errorf("llm: Anthropic stopped: %s", finish)
	}
	s.send(StreamChunk{Type: ChunkFinish, FinishReason: finish, Usage: usage})
	return nil
}
