package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"harness/internal/session"
)

// streamRequest 在本次请求开始前固定配置，凭据只用于本次 HTTP 请求。
type streamRequest struct {
	definition model
	protocol   string
	baseURL    string
	token      string
	headers    map[string]string
	system     string
	messages   []modelMessage
	tools      []toolDefinition
	toolChoice string
	options    map[string]any
}

type streamSink struct {
	ctx     context.Context
	output  chan StreamChunk
	request streamRequest
}

func (s streamSink) send(chunk StreamChunk) bool {
	select {
	case <-s.ctx.Done():
		return false
	case s.output <- chunk:
		return true
	}
}

func (s streamSink) continuation(data json.RawMessage) {
	s.send(StreamChunk{Type: ChunkContinuation, Continuation: &session.ModelContinuation{
		Provider: s.request.definition.Provider, Model: s.request.definition.ID, API: s.request.protocol, Data: data,
	}})
}

func (r streamRequest) accepts(value *session.ModelContinuation) bool {
	return value != nil && value.Provider == r.definition.Provider && value.Model == r.definition.ID && value.API == r.protocol
}

func (s streamSink) call(id, name, arguments string, extra json.RawMessage) error {
	if id == "" || name == "" {
		return fmt.Errorf("llm: tool call needs id and name")
	}
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	// 完整事件中的非法 JSON 仍交给工具层返回错误，确保每个调用都有对应结果。
	chunk := StreamChunk{Type: ChunkToolCall, ToolCallID: id, ToolName: name, ToolInput: arguments}
	if len(extra) > 0 {
		chunk.Continuation = &session.ModelContinuation{Provider: s.request.definition.Provider, Model: s.request.definition.ID, API: s.request.protocol, Data: extra}
	}
	s.send(chunk)
	return nil
}

func startStream(ctx context.Context, request streamRequest) <-chan StreamChunk {
	output := make(chan StreamChunk)
	sink := streamSink{ctx: ctx, output: output, request: request}
	go func() {
		defer close(output)
		var err error
		switch request.protocol {
		case "openai-chat":
			err = sink.chat()
		case "openai-responses", "openai-codex":
			err = sink.responses()
		case "anthropic":
			err = sink.anthropic()
		default:
			err = fmt.Errorf("llm: unsupported protocol %q", request.protocol)
		}
		if err != nil {
			sink.send(StreamChunk{Type: ChunkError, Error: err})
		}
	}()
	return output
}
