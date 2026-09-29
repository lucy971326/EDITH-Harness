package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

func (r streamRequest) responsesBody() map[string]any {
	input := []any{}
	for _, message := range r.messages {
		if message.Role == RoleSystem {
			var system strings.Builder
			for _, part := range message.Content {
				if part.Type != PartText || part.Text == "" {
					continue
				}
				if system.Len() > 0 {
					system.WriteByte('\n')
				}
				system.WriteString(part.Text)
			}
			if system.Len() > 0 {
				input = append(input, map[string]any{"role": RoleSystem, "content": system.String()})
			}
			continue
		}
		content := []any{}
		for _, part := range message.Content {
			switch part.Type {
			case PartText:
				if part.Text == "" {
					continue
				}
				kind := "input_text"
				if message.Role == RoleAssistant {
					kind = "output_text"
				}
				content = append(content, map[string]any{"type": kind, "text": part.Text})
			case PartImage:
				content = append(content, map[string]any{"type": "input_image", "image_url": part.URL})
			case PartReasoning:
				if len(content) > 0 {
					input = append(input, map[string]any{"role": message.Role, "content": content})
					content = nil
				}
				if r.accepts(part.Continuation) {
					input = append(input, part.Continuation.Data)
				}
			case PartToolCall:
				if len(content) > 0 {
					input = append(input, map[string]any{"role": message.Role, "content": content})
					content = nil
				}
				input = append(input, map[string]any{"type": "function_call", "call_id": part.ToolCallID, "name": part.ToolName, "arguments": string(part.ToolInput)})
			case PartToolResult:
				input = append(input, map[string]any{"type": "function_call_output", "call_id": part.ToolCallID, "output": part.ToolOutput})
			}
		}
		if len(content) > 0 {
			input = append(input, map[string]any{"role": message.Role, "content": content})
		}
	}
	body := map[string]any{"model": r.definition.ID, "input": input, "stream": true, "store": false}
	if r.system != "" {
		body["instructions"] = r.system
	}
	if r.definition.MaxOutput > 0 && r.protocol != "openai-codex" {
		body["max_output_tokens"] = r.definition.MaxOutput
	}
	tools := []any{}
	for _, tool := range r.tools {
		tools = append(tools, map[string]any{"type": "function", "name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema, "strict": false})
	}
	if len(tools) > 0 {
		body["tools"] = tools
		if r.toolChoice != "" {
			body["tool_choice"] = r.toolChoice
		}
	}
	if effort, ok := r.options["reasoning_effort"]; ok {
		body["reasoning"] = map[string]any{"effort": effort, "summary": "auto"}
		body["include"] = []string{"reasoning.encrypted_content"}
	}
	if r.protocol == "openai-codex" {
		if r.system == "" {
			body["instructions"] = "You are a helpful assistant."
		}
		body["parallel_tool_calls"] = true
		body["include"] = []string{"reasoning.encrypted_content"}
	}
	return body
}

func (s streamSink) responses() error {
	client := s.request.openAIClient()
	var response *http.Response
	stream := client.Responses.NewStreaming(s.ctx, param.Override[responses.ResponseNewParams](s.request.responsesBody()), option.WithResponseInto(&response))
	defer stream.Close()
	finished := false
	pending := map[int]*pendingCall{}
	textDeltas := map[int]bool{}
	reasoningDeltas := map[int]bool{}
	for stream.Next() {
		var event struct {
			Type      string          `json:"type"`
			Message   string          `json:"message"`
			Index     int             `json:"output_index"`
			Delta     string          `json:"delta"`
			Arguments string          `json:"arguments"`
			Item      json.RawMessage `json:"item"`
			Response  struct {
				Status string `json:"status"`
				Usage  struct {
					Input   int `json:"input_tokens"`
					Output  int `json:"output_tokens"`
					Details struct {
						Cached int `json:"cached_tokens"`
					} `json:"input_tokens_details"`
				} `json:"usage"`
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
				Incomplete struct {
					Reason string `json:"reason"`
				} `json:"incomplete_details"`
			} `json:"response"`
		}
		err := json.Unmarshal([]byte(stream.Current().RawJSON()), &event)
		if err != nil {
			return err
		}
		switch event.Type {
		case "response.output_text.delta":
			textDeltas[event.Index] = true
			if !s.send(StreamChunk{Type: ChunkText, Text: event.Delta}) {
				return s.ctx.Err()
			}
		case "response.refusal.delta":
			textDeltas[event.Index] = true
			if !s.send(StreamChunk{Type: ChunkText, Text: event.Delta}) {
				return s.ctx.Err()
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			reasoningDeltas[event.Index] = true
			if !s.send(StreamChunk{Type: ChunkReasoning, Text: event.Delta}) {
				return s.ctx.Err()
			}
		case "response.output_item.added", "response.output_item.done":
			var item struct {
				Type      string `json:"type"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
				Content   []struct {
					Type    string `json:"type"`
					Text    string `json:"text"`
					Refusal string `json:"refusal"`
				} `json:"content"`
				Summary []struct {
					Text string `json:"text"`
				} `json:"summary"`
			}
			if len(event.Item) > 0 {
				err = json.Unmarshal(event.Item, &item)
				if err != nil {
					return err
				}
			}
			if event.Type == "response.output_item.done" {
				if item.Type == "reasoning" {
					if !reasoningDeltas[event.Index] {
						if len(item.Summary) > 0 {
							for _, summary := range item.Summary {
								if summary.Text != "" && !s.send(StreamChunk{Type: ChunkReasoning, Text: summary.Text}) {
									return s.ctx.Err()
								}
							}
						} else {
							for _, content := range item.Content {
								if content.Text != "" && !s.send(StreamChunk{Type: ChunkReasoning, Text: content.Text}) {
									return s.ctx.Err()
								}
							}
						}
					}
					s.continuation(event.Item)
				}
				if item.Type == "message" && !textDeltas[event.Index] {
					for _, content := range item.Content {
						value := content.Text
						if content.Type == "refusal" {
							value = content.Refusal
						}
						if value != "" && !s.send(StreamChunk{Type: ChunkText, Text: value}) {
							return s.ctx.Err()
						}
					}
				}
			}
			if item.Type == "function_call" {
				call := pending[event.Index]
				if call == nil {
					call = &pendingCall{}
					pending[event.Index] = call
				}
				if item.CallID != "" {
					call.id = item.CallID
				}
				if item.Name != "" {
					call.name = item.Name
				}
				if item.Arguments != "" {
					call.arguments = item.Arguments
				}
			}
			if event.Type == "response.output_item.done" {
				if call := pending[event.Index]; call != nil {
					err = s.call(call.id, call.name, call.arguments, nil)
					if err != nil {
						return err
					}
					delete(pending, event.Index)
				}
			}
		case "response.function_call_arguments.delta":
			if call := pending[event.Index]; call != nil {
				call.arguments += event.Delta
			}
		case "response.function_call_arguments.done":
			if call := pending[event.Index]; call != nil {
				call.arguments = event.Arguments
			}
		case "response.completed", "response.incomplete", "response.done":
			if len(pending) > 0 {
				return fmt.Errorf("llm: response ended with unfinished tool calls")
			}
			if event.Type == "response.done" && event.Response.Status != "" && event.Response.Status != "completed" && event.Response.Status != "incomplete" {
				return fmt.Errorf("llm: Codex response ended with %s: %s", event.Response.Status, event.Response.Error.Message)
			}
			reason := "stop"
			if event.Type == "response.incomplete" || event.Type == "response.done" && event.Response.Status == "incomplete" {
				if event.Response.Incomplete.Reason != "max_output_tokens" {
					return fmt.Errorf("llm: incomplete response: %s", event.Response.Incomplete.Reason)
				}
				reason = "length"
			}
			u := event.Response.Usage
			s.send(StreamChunk{Type: ChunkFinish, FinishReason: reason, Usage: Usage{InputTokens: max(0, u.Input-u.Details.Cached), OutputTokens: u.Output, CacheReadTokens: u.Details.Cached}})
			finished = true
		case "error":
			return fmt.Errorf("llm: response failed: %s", event.Message)
		case "response.failed", "response.cancelled":
			return fmt.Errorf("llm: response failed (%s): %s", event.Type, event.Response.Error.Message)
		}
	}
	if err := stream.Err(); err != nil {
		return openAIStreamError(err, response)
	}
	if !finished {
		return fmt.Errorf("llm: response stream interrupted before completion")
	}
	return nil
}
