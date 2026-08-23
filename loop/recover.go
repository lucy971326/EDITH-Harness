package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"harness/session"
	"harness/tools"
)

// recoverFromLedger 拿旧账重建 agent：找回没领的待办、补齐悬空的账。
// 原则：账说什么就是什么，绝不猜、绝不自动重跑可能已有副作用的工具。
func recoverFromLedger(agent *Conversation, seed []session.Event) error {
	var (
		delivered        = make(map[string]session.Event) // 投递了、还没领出的
		calls            = make(map[string]bool)          // 要调了、还没回话的调用
		started          = make(map[string]bool)          // 真的开跑过的调用
		pendingChunkSeqs []int                            // 悬空的流式片段（说了半句没有定稿）
		pendingChunkText strings.Builder
		openTurn         bool
		openStep         bool
	)

	for _, event := range seed {
		switch event.Kind {
		case session.KindDeliver:
			var data session.DeliverData
			err := json.Unmarshal(event.Data, &data)
			if err != nil {
				return fmt.Errorf("第 %d 笔投递解不开：%w", event.Seq, err)
			}
			agent.inbox.rememberDeliveryID(data.ID)
			delivered[data.ID] = event
		case session.KindClaim:
			var data session.ClaimData
			err := json.Unmarshal(event.Data, &data)
			if err != nil {
				return fmt.Errorf("第 %d 笔领出解不开：%w", event.Seq, err)
			}
			delete(delivered, data.ID)
		case session.KindToolCall:
			var data session.ToolCallData
			err := json.Unmarshal(event.Data, &data)
			if err != nil {
				return fmt.Errorf("第 %d 笔调用解不开：%w", event.Seq, err)
			}
			calls[data.ID] = true
		case session.KindToolStart:
			var data session.ToolStartData
			err := json.Unmarshal(event.Data, &data)
			if err != nil {
				return fmt.Errorf("第 %d 笔开跑解不开：%w", event.Seq, err)
			}
			started[data.CallID] = true
		case session.KindToolResult:
			var data session.ToolResultData
			err := json.Unmarshal(event.Data, &data)
			if err != nil {
				return fmt.Errorf("第 %d 笔回话解不开：%w", event.Seq, err)
			}
			delete(calls, data.CallID)
			delete(started, data.CallID)
		case session.KindChunk:
			var data session.ChunkData
			err := json.Unmarshal(event.Data, &data)
			if err != nil {
				return fmt.Errorf("第 %d 笔片段解不开：%w", event.Seq, err)
			}
			pendingChunkSeqs = append(pendingChunkSeqs, event.Seq)
			pendingChunkText.WriteString(data.Delta)
		case session.KindAssistantFinal:
			// 定稿把之前的悬空片段收编了。
			pendingChunkSeqs = nil
			pendingChunkText.Reset()
		case session.KindTurnStart:
			openTurn = true
		case session.KindTurnEnd:
			openTurn = false
		case session.KindStepStart:
			openStep = true
		case session.KindStepEnd:
			openStep = false
		}
	}

	// 悬空的工具：补终局（走 tools 的补账口，规矩不破）。
	for callID := range calls {
		if started[callID] {
			// 副作用可能已发生：说不知道，让模型自己裁决。
			err := tools.RecoverDangling(agent.book, callID, tools.ResultUnknown,
				"这个操作断在半路，结果不明：副作用可能已发生也可能没发生。你自己判断要不要重做；拿不准就先问我。")
			if err != nil {
				return err
			}
			continue
		}
		// 没开跑就是没开跑：放心跳过，模型可重试。
		err := tools.RecoverDangling(agent.book, callID, session.ResultSkipped,
			"断在开跑前，什么都没发生，可以直接重试。")
		if err != nil {
			return err
		}
	}

	// 悬空的半句话：固化成被打断的定稿——只落一条，字一个不丢。
	if len(pendingChunkSeqs) > 0 {
		_, err := agent.book.RecordAssistantFinal(session.AssistantFinalData{
			Text:        pendingChunkText.String(),
			Interrupted: true,
		}, pendingChunkSeqs)
		if err != nil {
			return err
		}
	}

	// 悬空的步和轮：补上收口。
	if openStep {
		_, err := agent.book.RecordStepEnd()
		if err != nil {
			return err
		}
	}
	if openTurn {
		_, err := agent.book.RecordTurnEnd()
		if err != nil {
			return err
		}
	}

	// 没领出的投递放回收件箱——用户的消息一条不丢，重启后接着办。
	for _, event := range seed {
		if event.Kind != session.KindDeliver {
			continue
		}
		var data session.DeliverData
		err := json.Unmarshal(event.Data, &data)
		if err != nil {
			return err
		}
		if _, waiting := delivered[data.ID]; waiting {
			agent.inbox.restore(delivery{ID: data.ID, Text: data.Text}, data.Target)
		}
	}

	// 有找回的待办就唤醒接着干，没有就安安静静待命。
	if agent.inbox.pending() {
		agent.inbox.ring()
	}
	return nil
}
