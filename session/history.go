package session

import (
	"encoding/json"
	"strings"

	"harness/chat"
)

// historyCache 是算模型历史的内部状态：有效名单 + 已算到哪 + 算出的消息。
// 一句话规则：被摘要收编的不算；连续 chunk 折叠成一条助手消息。
type historyCache struct {
	alive     map[int]bool    // 有效名单：编号 -> 还没被取代
	processed int             // 已算到第几笔（增量用）
	messages  []chat.Message  // 已收口的消息
	textBuf   strings.Builder // 未收口的助手文字（流式片段攒着）
	callsBuf  []chat.ToolCall // 同一条助手消息里攒的工具调用
	rebuilds  int             // 全量重算次数，测试观察用
}

func newHistoryCache() *historyCache {
	return &historyCache{alive: make(map[int]bool)}
}

// rebuild 全量重算：有取代发生时缓存作废，从头逐笔重放。
func (h *historyCache) rebuild(events []Event) {
	h.rebuilds++
	h.alive = make(map[int]bool)
	for _, event := range events {
		h.alive[event.Seq] = true
	}
	for _, event := range events {
		for _, replaced := range event.Replaces {
			delete(h.alive, replaced)
		}
	}

	h.processed = 0
	h.messages = nil
	h.textBuf.Reset()
	h.callsBuf = nil
	for _, event := range events {
		if h.alive[event.Seq] {
			h.step(event)
		}
		h.processed = event.Seq
	}
}

// note 记一笔新账后更新缓存：有取代就全量重算，否则增量走一步。
func (h *historyCache) note(event Event, events []Event) {
	if len(event.Replaces) > 0 {
		h.rebuild(events)
		return
	}
	h.alive[event.Seq] = true
	h.step(event)
	h.processed = event.Seq
}

// step 处理一笔账，按种类组装或跳过。
// 收口时机：产消息的事件到达前、以及 step/turn 结束时——一个 step 就是一次完整的模型回复。
func (h *historyCache) step(event Event) {
	switch event.Kind {
	case KindUserMessage:
		h.flush()
		data := decodeInto(&UserMessageData{}, event.Data)
		h.messages = append(h.messages, chat.Message{Role: "user", Text: data.Text})

	case KindChunk:
		data := decodeInto(&ChunkData{}, event.Data)
		h.textBuf.WriteString(data.Delta)

	case KindAssistantFinal:
		// 定稿收场：它引用的素材已被有效名单收编，这里只管产出定稿。
		// 残余没被收编的素材（异常路径）先收口，别混进定稿。
		h.flush()
		data := decodeInto(&AssistantFinalData{}, event.Data)
		h.messages = append(h.messages, chat.Message{
			Role:        "assistant",
			Text:        data.Text,
			Interrupted: data.Interrupted,
		})

	case KindToolCall:
		data := decodeInto(&ToolCallData{}, event.Data)
		h.callsBuf = append(h.callsBuf, chat.ToolCall{
			ID:       data.ID,
			Name:     data.Name,
			Argument: append(json.RawMessage(nil), data.Argument...),
		})

	case KindToolResult:
		h.flush()
		data := decodeInto(&ToolResultData{}, event.Data)
		h.messages = append(h.messages, chat.Message{
			Role: "tool",
			Result: &chat.ToolResult{
				CallID: data.CallID,
				Output: data.Output,
				Status: data.Status,
			},
		})

	case KindSummary:
		h.flush()
		data := decodeInto(&SummaryData{}, event.Data)
		h.messages = append(h.messages, chat.Message{Role: "user", Text: data.Text})

	case KindStepEnd, KindTurnEnd:
		// 回复收口：流式片段到此为止，后面来的字属于新的一条。
		h.flush()

	default:
		// turn/step 开始、投递、领出、快照、tool/start：账上有，模型看不见。
	}
}

// flush 把攒的助手文字和工具调用收口成一条助手消息。
func (h *historyCache) flush() {
	if h.textBuf.Len() == 0 && len(h.callsBuf) == 0 {
		return
	}
	h.messages = append(h.messages, chat.Message{
		Role:  "assistant",
		Text:  h.textBuf.String(),
		Calls: h.callsBuf,
	})
	h.textBuf.Reset()
	h.callsBuf = nil
}

// ModelHistory 返回当前模型该看到的消息。
// 只读不动状态：没收口的尾部缓冲临时拼成一条附加，后续片段还能续上。
func (h *historyCache) modelHistory() []chat.Message {
	messages := make([]chat.Message, len(h.messages))
	copy(messages, h.messages)

	if h.textBuf.Len() > 0 || len(h.callsBuf) > 0 {
		messages = append(messages, chat.Message{
			Role:  "assistant",
			Text:  h.textBuf.String(),
			Calls: append([]chat.ToolCall(nil), h.callsBuf...),
		})
	}
	return messages
}

// decodeInto 把账数据解成结构体；解不动说明账坏了（记账时已体检过，这里是防御）。
func decodeInto[T any](blank *T, raw json.RawMessage) *T {
	value := new(T)
	err := json.Unmarshal(raw, value)
	if err != nil {
		panic(err)
	}
	return value
}
