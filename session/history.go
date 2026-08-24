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
	textBuf   strings.Builder // 没有定稿的流式文字
	pending   *chat.Message   // 已定稿、等候其后工具调用的助手消息
	results   []chat.Message  // 等候助手消息收口的工具结果
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
	h.pending = nil
	h.results = nil
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
		// 定稿先不立刻放进历史。工具调用在它之后入账，必须合成同一条消息。
		h.flush()
		data := decodeInto(&AssistantFinalData{}, event.Data)
		h.pending = &chat.Message{
			Role:        "assistant",
			Text:        data.Text,
			Thinking:    data.Thinking,
			Interrupted: data.Interrupted,
		}

	case KindToolCall:
		data := decodeInto(&ToolCallData{}, event.Data)
		if h.pending == nil {
			h.pending = &chat.Message{Role: "assistant", Text: h.textBuf.String()}
			h.textBuf.Reset()
		}
		h.pending.Calls = append(h.pending.Calls, chat.ToolCall{
			ID:       data.ID,
			Name:     data.Name,
			Argument: append(json.RawMessage(nil), data.Argument...),
		})

	case KindToolResult:
		data := decodeInto(&ToolResultData{}, event.Data)
		result := chat.Message{
			Role: "tool",
			Result: &chat.ToolResult{
				CallID: data.CallID,
				Output: data.Output,
				Status: data.Status,
			},
		}
		if h.pending != nil {
			h.results = append(h.results, result)
			return
		}
		h.messages = append(h.messages, result)

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

// flush 把已定稿的助手消息及没有定稿的流式尾巴收口。
func (h *historyCache) flush() {
	if h.pending != nil {
		message := *h.pending
		message.Calls = append([]chat.ToolCall(nil), h.pending.Calls...)
		h.messages = append(h.messages, message)
		h.pending = nil
	}
	if len(h.results) > 0 {
		h.messages = append(h.messages, h.results...)
		h.results = nil
	}
	if h.textBuf.Len() > 0 {
		h.messages = append(h.messages, chat.Message{Role: "assistant", Text: h.textBuf.String()})
		h.textBuf.Reset()
	}
}

// ModelHistory 返回当前模型该看到的消息。
// 只读不动状态：没收口的尾部缓冲临时拼成一条附加，后续片段还能续上。
func (h *historyCache) modelHistory() []chat.Message {
	messages := make([]chat.Message, len(h.messages))
	copy(messages, h.messages)

	if h.pending != nil {
		message := *h.pending
		message.Calls = append([]chat.ToolCall(nil), h.pending.Calls...)
		messages = append(messages, message)
	}
	if len(h.results) > 0 {
		messages = append(messages, h.results...)
	}
	if h.textBuf.Len() > 0 {
		messages = append(messages, chat.Message{
			Role: "assistant",
			Text: h.textBuf.String(),
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
