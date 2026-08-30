package react

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zendev-sh/goai/provider"

	"harness/kernel/llm"
	"harness/kernel/loops"
	"harness/kernel/session"
	"harness/kernel/tools"
)

// 活对象。登记在 loops 插槽中的默认 ReAct 程序。
type reactLoop struct {
	llm   *llm.Client
	tools tools.Tools
}

func (l *reactLoop) Definition() loops.Definition {
	return loops.Definition{
		Kind:        "react",
		Description: "Call an LLM, execute requested tools, and continue until it answers.",
	}
}

func (l *reactLoop) Run(ctx context.Context, invocation loops.Invocation) error {
	if invocation.Emit == nil {
		return fmt.Errorf("react: nil Emit")
	}
	if invocation.Checkpoint == nil {
		return fmt.Errorf("react: nil Checkpoint")
	}

	invocation.ToolNames = append([]string(nil), invocation.ToolNames...)
	definitions, err := l.tools.Definitions(invocation.ToolNames)
	if err != nil {
		return err
	}
	history := append([]session.Message(nil), invocation.History...)

	for {
		assistant, calls, err := l.request(ctx, invocation, history, definitions)
		if err != nil {
			return err
		}
		err = invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, Message: &assistant})
		if err != nil {
			return err
		}
		history = append(history, assistant)

		for _, call := range calls {
			resultMessage, err := l.execute(ctx, invocation, call)
			if err != nil {
				return err
			}
			history = append(history, resultMessage)
		}

		phase := loops.CheckpointContinue
		if len(calls) == 0 {
			phase = loops.CheckpointFinal
		}
		steers, err := invocation.Checkpoint(ctx, phase)
		if err != nil {
			return err
		}
		history = append(history, steers...)
		if len(calls) == 0 && len(steers) == 0 {
			return nil
		}
	}
}

func (l *reactLoop) request(
	ctx context.Context,
	invocation loops.Invocation,
	history []session.Message,
	definitions []tools.Definition,
) (session.Message, []session.ToolCall, error) {
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := l.llm.Stream(requestCtx, invocation.LLMConfig, llm.Input{
		System:  invocation.SystemPrompt,
		History: history,
		Tools:   definitions,
	})
	if err != nil {
		return session.Message{}, nil, err
	}

	message := session.Message{Role: session.RoleAssistant}
	var calls []session.ToolCall
	for {
		select {
		case <-ctx.Done():
			return session.Message{}, nil, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				if err := ctx.Err(); err != nil {
					return session.Message{}, nil, err
				}
				return message, calls, nil
			}
			switch chunk.Type {
			case provider.ChunkReasoning:
				appendTextBlock(&message, "reasoning", chunk.Text)
				err = invocation.Emit(ctx, loops.Event{Kind: loops.EventReasoningDelta, Text: chunk.Text})
				if err != nil {
					return session.Message{}, nil, err
				}
			case provider.ChunkText:
				appendTextBlock(&message, "text", chunk.Text)
				err = invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, Text: chunk.Text})
				if err != nil {
					return session.Message{}, nil, err
				}
			case provider.ChunkToolCall:
				if chunk.ToolCallID == "" || chunk.ToolName == "" {
					return session.Message{}, nil, fmt.Errorf("react: tool call needs id and name")
				}
				call := session.ToolCall{
					ID:   chunk.ToolCallID,
					Name: chunk.ToolName,
					Args: chunk.ToolInput,
				}
				calls = append(calls, call)
				message.Blocks = append(message.Blocks, session.Block{Kind: "tool-call", Tool: &call})
			case provider.ChunkError:
				if chunk.Error == nil {
					return session.Message{}, nil, fmt.Errorf("react: model stream failed")
				}
				return session.Message{}, nil, chunk.Error
			}
		}
	}
}

func (l *reactLoop) execute(
	ctx context.Context,
	invocation loops.Invocation,
	call session.ToolCall,
) (session.Message, error) {
	err := invocation.Emit(ctx, loops.Event{
		Kind: loops.EventToolStarted,
		Tool: &loops.ToolEvent{ID: call.ID, Name: call.Name},
	})
	if err != nil {
		return session.Message{}, err
	}

	result, err := l.tools.Call(ctx, tools.Call{
		Name:      call.Name,
		Arguments: json.RawMessage(call.Args),
		Workspace: invocation.Workspace,
		Allow:     invocation.ToolNames,
	})
	if err != nil {
		return session.Message{}, err
	}
	content := result.Content
	if result.IsError {
		content = errorText(content)
	}
	message := session.Message{
		Role: session.RoleTool,
		Blocks: []session.Block{{
			Kind: "tool-result",
			Result: &session.ToolResult{
				ID:      call.ID,
				Name:    call.Name,
				Content: content,
				IsError: result.IsError,
			},
		}},
	}
	err = invocation.Emit(ctx, loops.Event{Kind: loops.EventMessage, Message: &message})
	if err != nil {
		return session.Message{}, err
	}
	err = invocation.Emit(ctx, loops.Event{
		Kind: loops.EventToolFinished,
		Tool: &loops.ToolEvent{ID: call.ID, Name: call.Name, IsError: result.IsError},
	})
	if err != nil {
		return session.Message{}, err
	}
	return message, nil
}

func appendTextBlock(message *session.Message, kind string, text string) {
	if text == "" {
		return
	}
	last := len(message.Blocks) - 1
	if last >= 0 && message.Blocks[last].Kind == kind {
		message.Blocks[last].Text += text
		return
	}
	message.Blocks = append(message.Blocks, session.Block{Kind: kind, Text: text})
}

func errorText(content string) string {
	if strings.HasPrefix(content, "error: ") {
		return content
	}
	return "error: " + content
}
