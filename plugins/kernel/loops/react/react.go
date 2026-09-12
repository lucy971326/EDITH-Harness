package react

import (
	"context"
	"encoding/json"
	"errors"
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
	definitions, err := l.tools.Definitions(ctx, invocation.Workspace, invocation.ToolNames)
	if err != nil {
		return err
	}
	history := append([]session.Message(nil), invocation.History...)

	for {
		assistant, assistantID, calls, err := l.request(ctx, invocation, history, definitions)
		if err != nil {
			return err
		}
		if assistantID == "" {
			return nil
		}
		err = invocation.Emit(context.WithoutCancel(ctx), loops.Event{Kind: loops.EventMessage, EntryID: assistantID, Message: &assistant})
		if err != nil {
			return err
		}
		history = append(history, assistant)

		for index, call := range calls {
			err = ctx.Err()
			if err != nil {
				return l.finishUnexecuted(ctx, invocation, assistantID, assistant, calls[index:], err)
			}
			resultMessage, err := l.execute(ctx, invocation, assistantID, call, blockForCall(assistant, call.ID))
			if err != nil {
				// 当前取消结果已成功落账后，补齐这批尚未执行的调用。
				if resultMessage.Role == session.RoleTool && ctx.Err() != nil {
					return l.finishUnexecuted(ctx, invocation, assistantID, assistant, calls[index+1:], err)
				}
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
) (session.Message, string, []session.ToolCall, error) {
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := l.llm.Stream(requestCtx, invocation.LLMConfig, llm.Input{
		System:  invocation.SystemPrompt,
		History: history,
		Tools:   definitions,
	})
	if err != nil {
		return session.Message{}, "", nil, err
	}

	entryID, err := session.NewEntryID()
	if err != nil {
		return session.Message{}, "", nil, err
	}
	err = invocation.Emit(ctx, loops.Event{Kind: loops.EventMessageStarted, EntryID: entryID})
	if err != nil {
		return session.Message{}, "", nil, err
	}

	message := session.Message{Role: session.RoleAssistant}
	var calls []session.ToolCall
	var usage provider.Usage
	fail := func(cause error) (session.Message, string, []session.ToolCall, error) {
		return l.failRequest(ctx, invocation, entryID, message, cause)
	}
	for {
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case chunk, ok := <-stream:
			if !ok {
				if err := ctx.Err(); err != nil {
					return fail(err)
				}
				if len(message.Blocks) == 0 && len(calls) == 0 {
					return session.Message{}, "", nil, nil
				}
				err = l.emitUsage(ctx, invocation, entryID, usage)
				if err != nil {
					return session.Message{}, "", nil, err
				}
				return message, entryID, calls, nil
			}
			switch chunk.Type {
			case provider.ChunkReasoning:
				blockSeq := appendTextBlock(&message, "reasoning", chunk.Text)
				err = invocation.Emit(ctx, loops.Event{Kind: loops.EventReasoningDelta, EntryID: entryID, BlockSeq: blockSeq, Text: chunk.Text})
				if err != nil {
					return session.Message{}, "", nil, err
				}
			case provider.ChunkText:
				blockSeq := appendTextBlock(&message, "text", chunk.Text)
				err = invocation.Emit(ctx, loops.Event{Kind: loops.EventTextDelta, EntryID: entryID, BlockSeq: blockSeq, Text: chunk.Text})
				if err != nil {
					return session.Message{}, "", nil, err
				}
			case provider.ChunkToolCall:
				if chunk.ToolCallID == "" || chunk.ToolName == "" {
					return session.Message{}, "", nil, fmt.Errorf("react: tool call needs id and name")
				}
				call := session.ToolCall{
					ID:   chunk.ToolCallID,
					Name: chunk.ToolName,
					Args: chunk.ToolInput,
				}
				calls = append(calls, call)
				message.Blocks = append(message.Blocks, session.Block{Kind: "tool-call", Tool: &call})
			case provider.ChunkFinish:
				usage = chunk.Usage
			case provider.ChunkError:
				if chunk.Error == nil {
					return fail(fmt.Errorf("react: model stream failed"))
				}
				return fail(chunk.Error)
			}
		}
	}
}

func (l *reactLoop) failRequest(ctx context.Context, invocation loops.Invocation, entryID string, message session.Message, cause error) (session.Message, string, []session.ToolCall, error) {
	incomplete, ok := incompleteAssistant(message)
	if !ok {
		return session.Message{}, "", nil, cause
	}
	err := invocation.Emit(context.WithoutCancel(ctx), loops.Event{Kind: loops.EventMessage, EntryID: entryID, Message: &incomplete})
	if err != nil {
		return session.Message{}, "", nil, errors.Join(cause, err)
	}
	return session.Message{}, "", nil, cause
}

func (l *reactLoop) emitUsage(ctx context.Context, invocation loops.Invocation, entryID string, usage provider.Usage) error {
	return invocation.Emit(ctx, loops.Event{
		Kind:    loops.EventUsage,
		EntryID: entryID,
		Usage: &loops.Usage{
			InputTokens:     usage.InputTokens,
			CacheReadTokens: usage.CacheReadTokens,
			ContextWindow:   l.llm.ContextWindow(invocation.LLMConfig.Model),
		},
	})
}

func (l *reactLoop) execute(
	ctx context.Context,
	invocation loops.Invocation,
	assistantID string,
	call session.ToolCall,
	blockSeq uint64,
) (session.Message, error) {
	finishCtx := context.WithoutCancel(ctx)
	err := invocation.Emit(finishCtx, loops.Event{
		Kind:     loops.EventToolStarted,
		EntryID:  assistantID,
		BlockSeq: blockSeq,
		Tool:     &loops.ToolEvent{ID: call.ID, Name: call.Name},
	})
	if err != nil {
		return session.Message{}, err
	}

	var inputSignal <-chan struct{}
	if invocation.InputSignal != nil {
		inputSignal = invocation.InputSignal()
	}
	result, err := l.tools.Call(ctx, tools.Call{
		Name:        call.Name,
		Arguments:   json.RawMessage(call.Args),
		Workspace:   invocation.Workspace,
		Allow:       invocation.ToolNames,
		SessionID:   invocation.SessionID,
		RunID:       invocation.RunID,
		ToolCallID:  call.ID,
		InputSignal: inputSignal,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			message := cancelledToolMessage(call)
			resultID, idErr := session.NewEntryID()
			if idErr != nil {
				return session.Message{}, idErr
			}
			err = invocation.Emit(finishCtx, loops.Event{Kind: loops.EventMessage, EntryID: resultID, Message: &message})
			if err != nil {
				return session.Message{}, err
			}
			err = invocation.Emit(finishCtx, loops.Event{
				Kind:     loops.EventToolFinished,
				EntryID:  assistantID,
				BlockSeq: blockSeq,
				Tool:     &loops.ToolEvent{ID: call.ID, Name: call.Name, IsError: true},
			})
			if err != nil {
				return session.Message{}, err
			}
			return message, ctx.Err()
		}
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
	resultID, err := session.NewEntryID()
	if err != nil {
		return session.Message{}, err
	}
	err = invocation.Emit(finishCtx, loops.Event{Kind: loops.EventMessage, EntryID: resultID, Message: &message})
	if err != nil {
		return session.Message{}, err
	}
	err = invocation.Emit(finishCtx, loops.Event{
		Kind:     loops.EventToolFinished,
		EntryID:  assistantID,
		BlockSeq: blockSeq,
		Tool:     &loops.ToolEvent{ID: call.ID, Name: call.Name, IsError: result.IsError},
	})
	if err != nil {
		return session.Message{}, err
	}
	return message, nil
}

// finishUnexecuted 只补结果，不执行剩余工具；写入或通知失败时直接返回，不重复落账。
func (l *reactLoop) finishUnexecuted(ctx context.Context, invocation loops.Invocation, assistantID string, assistant session.Message, calls []session.ToolCall, cause error) error {
	finishCtx := context.WithoutCancel(ctx)
	for _, call := range calls {
		message := cancelledToolMessage(call)
		message.Blocks[0].Result.Content = "未执行：任务已停止"
		blockSeq := blockForCall(assistant, call.ID)
		resultID, err := session.NewEntryID()
		if err != nil {
			return err
		}
		err = invocation.Emit(finishCtx, loops.Event{Kind: loops.EventMessage, EntryID: resultID, Message: &message})
		if err != nil {
			return err
		}
		err = invocation.Emit(finishCtx, loops.Event{
			Kind: loops.EventToolFinished, EntryID: assistantID, BlockSeq: blockSeq,
			Tool: &loops.ToolEvent{ID: call.ID, Name: call.Name, IsError: true},
		})
		if err != nil {
			return err
		}
	}
	return cause
}

func incompleteAssistant(message session.Message) (session.Message, bool) {
	blocks := make([]session.Block, 0, len(message.Blocks))
	for _, block := range message.Blocks {
		if (block.Kind == "text" || block.Kind == "reasoning") && block.Text != "" {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return session.Message{}, false
	}
	return session.Message{Role: session.RoleAssistant, Blocks: blocks, Incomplete: true}, true
}

func cancelledToolMessage(call session.ToolCall) session.Message {
	return session.Message{
		Role: session.RoleTool,
		Blocks: []session.Block{{
			Kind: "tool-result",
			Result: &session.ToolResult{
				ID:      call.ID,
				Name:    call.Name,
				Content: "已取消",
				IsError: true,
			},
		}},
	}
}

func appendTextBlock(message *session.Message, kind string, text string) uint64 {
	if text == "" {
		return uint64(len(message.Blocks))
	}
	last := len(message.Blocks) - 1
	if last >= 0 && message.Blocks[last].Kind == kind {
		message.Blocks[last].Text += text
		return uint64(last + 1)
	}
	message.Blocks = append(message.Blocks, session.Block{Kind: kind, Text: text})
	return uint64(len(message.Blocks))
}

func blockForCall(message session.Message, callID string) uint64 {
	for index, block := range message.Blocks {
		if block.Kind == "tool-call" && block.Tool != nil && block.Tool.ID == callID {
			return uint64(index + 1)
		}
	}
	return 0
}

func errorText(content string) string {
	if strings.HasPrefix(content, "error: ") {
		return content
	}
	return "error: " + content
}
