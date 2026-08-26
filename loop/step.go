package loop

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"harness/chat"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

// Checkpoint 是步前检查站的链名：插件可以拒绝这批消息进上下文。
// 被拒的也留痕——关一个空轮记账，绝不静默吞掉。
const Checkpoint = "loop/pre-step"

// runStep 跑一步：领中途话 → 拍快照 → 清仓 → 问模型 → 定稿 → 调工具。
// 返回"还要不要下一步"。任何出口都把这一步收口（step/end），
// 账上不留悬空的步——恢复时少猜一件事。
func (d *driver) runStep() (bool, error) {
	c := d.conversation

	_, err := c.book.RecordStepStart()
	if err != nil {
		return false, err
	}
	stepOpen := true
	defer func() {
		if stepOpen {
			_, _ = c.book.RecordStepEnd()
		}
	}()

	for _, steering := range c.inbox.takeNextStep() {
		_ = c.claimAsUserMessage(steering)
	}

	// 组装最终请求：系统提示 + 账本投影出的历史。
	c.mu.Lock()
	config := c.config
	c.mu.Unlock()
	request := llm.Request{
		Provider: config.Provider,
		Model:    config.Model,
		Thinking: config.Thinking,
		Messages: buildMessages(config.SystemPrompt, c.book.ModelHistory()),
		Tools:    c.toolsReg.Schemas(c.sessionID),
	}
	snapshot, err := json.Marshal(request)
	if err != nil {
		return false, err
	}
	_, err = c.book.RecordSnapshot(snapshot)
	if err != nil {
		return false, err
	}

	// 危险边界：问模型前把攒着的账全部写完——万一这之后崩了，账知道断在哪。
	err = c.book.Flush()
	if err != nil {
		return false, err
	}

	// 边吐边记：UI 的逐字显示是白送的。
	var chunkSeqs []int
	chunkText := &strings.Builder{}
	ctx := c.stepContext()
	defer c.clearStepContext()
	reply, err := c.llmSvc.Stream(ctx, request, func(delta chat.Delta) {
		event, chunkErr := c.book.RecordChunk(delta.Text)
		if chunkErr != nil {
			log.Printf("loop: 记流式片段失败（账本异常，继续收字）：%v", chunkErr)
			return
		}
		chunkSeqs = append(chunkSeqs, event.Seq)
		chunkText.WriteString(delta.Text)
	})
	if err != nil {
		// 被取消：已收到的半句固化成被打断的定稿——只落一条，绝不丢字。
		if ctx.Err() != nil {
			if len(chunkSeqs) > 0 {
				_, _ = c.book.RecordAssistantFinal(session.AssistantFinalData{
					Text:        chunkText.String(),
					Interrupted: true,
				}, chunkSeqs)
			}
			return false, nil
		}
		return false, fmt.Errorf("问模型失败：%w", err)
	}

	_, err = c.book.RecordAssistantFinal(session.AssistantFinalData{
		Text:     reply.Text,
		Thinking: reply.Thinking,
		Usage:    reply.Usage,
	}, chunkSeqs)
	if err != nil {
		return false, err
	}

	if len(reply.Calls) == 0 {
		stepOpen = false
		_, err = c.book.RecordStepEnd()
		return false, err
	}

	// 模型要调工具：逐个过 M3 流水线，结果都入账后才走下一步。
	for _, call := range reply.Calls {
		result := c.toolsReg.ExecuteCall(ctx, c.scope, c.book, tools.Call{
			ID:       call.ID,
			Name:     call.Name,
			Argument: call.Argument,
			ScopeID:  c.sessionID,
		})
		if result.Status == tools.ResultUnknown {
			return false, fmt.Errorf("工具 %s 已开跑但结果不明，收轮待恢复", call.Name)
		}
		if ctx.Err() != nil {
			return false, nil
		}
	}

	stepOpen = false
	_, err = c.book.RecordStepEnd()
	if err != nil {
		return false, err
	}
	return true, nil
}

// buildMessages 拼最终发给模型的消息：系统提示在前、历史照账本。
func buildMessages(systemPrompt string, history []chat.Message) []chat.Message {
	var head []chat.Message
	if systemPrompt != "" {
		head = append(head, chat.Message{Role: "system", Text: systemPrompt})
	}
	return append(head, history...)
}
