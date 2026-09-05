package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zendev-sh/goai/provider"

	"harness/kernel/llm"
	"harness/kernel/session"
)

const compactInstruction = "请把到目前为止的对话压缩成一份后续可继续使用的摘要。保留目标、约束、已完成事项、关键结论和未完成工作。不要调用工具。只输出摘要正文。"

const (
	compactStepSeq  = uint64(1)
	compactBlockSeq = uint64(1)
)

type compactPreparation struct {
	sess         *session.Session
	systemPrompt string
	toolNames    []string
	workspace    string
	model        string
	effort       string
}

// Compact 占用空闲会话，用当前模型生成摘要并落账。失败或停止不改有效上下文。
func (r *Runner) Compact(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	prepared, err := r.prepareCompact(ctx, sessionID)
	if err != nil {
		return err
	}
	current := &liveRun{toolBlocks: make(map[string]toolBlock)}
	if err := r.begin(sessionID, current); err != nil {
		return err
	}
	go func() {
		defer r.end(sessionID, current)
		_ = r.runCompact(ctx, sessionID, current, prepared)
	}()
	return nil
}

func (r *Runner) prepareCompact(ctx context.Context, sessionID string) (compactPreparation, error) {
	sess, err := r.sessions.Get(sessionID)
	if err != nil {
		return compactPreparation{}, err
	}
	if len(sess.History()) == 0 {
		return compactPreparation{}, fmt.Errorf("runner: session %q has no history to compact", sessionID)
	}
	runSettings, err := r.settings.For(sessionID)
	if err != nil {
		return compactPreparation{}, err
	}
	if strings.TrimSpace(runSettings.Model) == "" {
		return compactPreparation{}, fmt.Errorf("runner: compact needs a model")
	}
	prepared, err := r.agents.Prepare(ctx, runSettings.AgentID, runSettings.Workspace)
	if err != nil {
		return compactPreparation{}, err
	}
	return compactPreparation{
		sess:         sess,
		systemPrompt: prepared.SystemPrompt,
		toolNames:    append([]string(nil), prepared.Tools...),
		workspace:    runSettings.Workspace,
		model:        runSettings.Model,
		effort:       runSettings.ReasoningEffort,
	}, nil
}

func (r *Runner) runCompact(parent context.Context, sessionID string, current *liveRun, prepared compactPreparation) (err error) {
	runID, err := newRunID()
	if err != nil {
		return err
	}
	if !current.startExclusive(runID) {
		return context.Canceled
	}
	runCtx, cancel := context.WithCancel(parent)
	current.setCancel(cancel)
	defer cancel()

	sess := prepared.sess
	entries := sess.Entries()
	current.setAfterEntrySeq(entries[len(entries)-1].Seq)
	if err := r.publish(runCtx, RunEvent{
		SessionID:     sessionID,
		RunID:         runID,
		Kind:          RunStarted,
		AfterEntrySeq: current.afterSeq(),
	}); err != nil {
		return err
	}
	defer func() {
		status := RunSucceeded
		if errors.Is(err, context.Canceled) {
			status = RunCancelled
		} else if err != nil {
			status = RunFailed
		}
		endErr := r.publish(context.Background(), RunEvent{
			SessionID:     sessionID,
			RunID:         runID,
			Kind:          RunEnded,
			AfterEntrySeq: current.afterSeq(),
			Status:        status,
			Error:         errorText(err),
		})
		if err == nil && endErr != nil {
			err = endErr
		}
	}()

	definitions, err := r.tools.Definitions(runCtx, prepared.workspace, prepared.toolNames)
	if err != nil {
		return err
	}
	history := append([]session.Message(nil), sess.History()...)
	history = append(history, session.Message{
		Role:   session.RoleUser,
		Blocks: []session.Block{{Kind: "text", Text: compactInstruction}},
	})
	input := llm.Input{
		System:  prepared.systemPrompt,
		History: history,
		Tools:   definitions,
	}
	if len(definitions) > 0 {
		input.ToolChoice = "none"
	}
	stream, err := r.llm.Stream(runCtx, llm.RunConfig{
		Model:           prepared.model,
		ReasoningEffort: prepared.effort,
	}, input)
	if err != nil {
		return err
	}

	text := ""
	sawToolCall := false
	finishReason := provider.FinishReason("")
	var usage provider.Usage
	for {
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case chunk, ok := <-stream:
			if !ok {
				return r.finishCompact(runCtx, sessionID, runID, sess, current, prepared.model, text, sawToolCall, finishReason, usage)
			}
			switch chunk.Type {
			case provider.ChunkReasoning:
				continue
			case provider.ChunkText:
				text += chunk.Text
				err = r.publish(runCtx, RunEvent{
					SessionID:     sessionID,
					RunID:         runID,
					Kind:          TextDelta,
					AfterEntrySeq: current.afterSeq(),
					StepSeq:       compactStepSeq,
					BlockSeq:      compactBlockSeq,
					Text:          chunk.Text,
				})
				if err != nil {
					return err
				}
			case provider.ChunkToolCall:
				sawToolCall = true
			case provider.ChunkStepFinish:
				if chunk.FinishReason != "" {
					finishReason = chunk.FinishReason
				}
			case provider.ChunkFinish:
				usage = chunk.Usage
				if chunk.FinishReason != "" {
					finishReason = chunk.FinishReason
				}
			case provider.ChunkError:
				if chunk.Error == nil {
					return fmt.Errorf("runner: compact stream failed")
				}
				return chunk.Error
			}
		}
	}
}

func (r *Runner) finishCompact(
	ctx context.Context,
	sessionID, runID string,
	sess *session.Session,
	current *liveRun,
	model, text string,
	sawToolCall bool,
	finishReason provider.FinishReason,
	usage provider.Usage,
) error {
	if sawToolCall {
		return fmt.Errorf("runner: compact requested a tool")
	}
	switch finishReason {
	case provider.FinishStop:
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("runner: compact produced empty summary")
		}
	case provider.FinishLength:
		return fmt.Errorf("runner: compact was truncated")
	case "":
		return fmt.Errorf("runner: compact finished without a stop reason")
	default:
		return fmt.Errorf("runner: compact finished with %s", finishReason)
	}

	blocks := []session.Block{{Kind: "summary", Text: text}}
	message := session.Message{RunID: runID, Role: session.RoleAssistant, Blocks: blocks}
	entry, err := sess.Append(message)
	if err != nil {
		return err
	}
	current.setAfterEntrySeq(entry.Seq)
	err = r.publish(ctx, RunEvent{
		SessionID:     sessionID,
		RunID:         runID,
		Kind:          Message,
		AfterEntrySeq: current.afterSeq(),
		StepSeq:       compactStepSeq,
		BlockSeq:      compactBlockSeq,
		Entry:         &entry,
	})
	if err != nil {
		return err
	}
	return r.publish(ctx, RunEvent{
		SessionID:     sessionID,
		RunID:         runID,
		Kind:          ContextUsage,
		AfterEntrySeq: current.afterSeq(),
		StepSeq:       compactStepSeq,
		Usage: &Usage{
			InputTokens:     usage.InputTokens,
			CacheReadTokens: usage.CacheReadTokens,
			ContextWindow:   r.llm.ContextWindow(model),
		},
	})
}
