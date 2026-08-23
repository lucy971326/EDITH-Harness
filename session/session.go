package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"

	"harness/chat"
)

// Broadcaster 是"记完账喊一嗓子"的出口。session 不依赖 core——
// 谁想收通知谁实现这个接口（core.App 天然满足），组装在 main 完成。
type Broadcaster interface {
	Broadcast(name string, payload any)
}

// EventAppended 是每记一笔后广播的事件名。
const EventAppended = "session/appended"

// Appended 是广播的负载：哪本账、记了哪笔。多本账并存时，听的人靠账本号分得清。
type Appended struct {
	SessionID string // 哪本账
	Event     Event  // 那笔账的副本
}

// ErrInBroadcast：广播回调里同步记账被拒绝——这时记会插账或死锁。
// 回调里要记账就投给 AppendLater，广播结束后自动补记。
var ErrInBroadcast = errors.New("广播进行中，回调里不能同步记账；请改用 AppendLater")

// Session 是一本账。
type Session struct {
	mu          sync.Mutex // 一本账一次只有一笔在写
	header      Header
	events      []Event
	journal     Journal
	broadcaster Broadcaster
	formats     *formatTable
	history     *historyCache

	queueMu sync.Mutex
	depth   int           // 广播深度，>0 表示正在广播或补记
	queue   []queuedEvent // 广播期间投递的补记，先进先出
}

// queuedEvent 是一笔等待补记的账。
type queuedEvent struct {
	kind     string
	data     any
	replaces []int
}

func newSession(header Header, journal Journal, broadcaster Broadcaster, formats *formatTable) *Session {
	return &Session{
		header:      header,
		journal:     journal,
		broadcaster: broadcaster,
		formats:     formats,
		history:     newHistoryCache(),
	}
}

// ID 返回账本号。
func (s *Session) ID() string {
	return s.header.ID
}

// Events 返回整本账的副本（改它不影响账）。
func (s *Session) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := make([]Event, len(s.events))
	for i, event := range s.events {
		events[i] = cloneEvent(event)
	}
	return events
}

// ModelHistory 返回模型该看到的消息：被摘要收编的不算，连续片段折叠成整句。
func (s *Session) ModelHistory() []chat.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.history.modelHistory()
}

// AppendEvent 记一笔插件自定义种类的账（种类须先在 Store 注册）。
// 内核种类在这里被拒绝——它们只走专用记账方法，一家管一种账。
func (s *Session) AppendEvent(kind string, data any) (Event, error) {
	if s.formats.isKernel(kind) {
		return Event{}, fmt.Errorf("种类 %s 是内核事件，走专用记账方法", kind)
	}
	format, known := s.formats.lookup(kind)
	if !known {
		return Event{}, fmt.Errorf("种类 %s 没注册，先到 Store.RegisterKind", kind)
	}
	return s.append(kind, data, nil, format.skipIfUnknown)
}

// AppendLater 投一笔异步账：不等结果，当前广播结束后自动补记并广播。
// 广播回调里要记账，用这个。
func (s *Session) AppendLater(kind string, data any) error {
	if s.formats.isKernel(kind) {
		return fmt.Errorf("种类 %s 是内核事件，走专用记账方法", kind)
	}
	s.queueMu.Lock()
	s.queue = append(s.queue, queuedEvent{kind: kind, data: data})
	s.queueMu.Unlock()
	return nil
}

// ---- 内核种类的专用记账方法：每家只准记自己的账 ----

// RecordUserMessage 记一句用户的话。
func (s *Session) RecordUserMessage(text string) (Event, error) {
	return s.append(KindUserMessage, UserMessageData{Text: text}, nil, false)
}

// RecordChunk 记模型吐的一小段字（流式素材，可攒批落盘，说不说不算数，看定稿）。
func (s *Session) RecordChunk(delta string) (Event, error) {
	return s.append(KindChunk, ChunkData{Delta: delta}, nil, false)
}

// RecordAssistantFinal 记模型一段话的定稿：收编它引用的素材（replaces 传那几个 chunk 的编号），
// 带上用量和是否被打断。说了什么、花了多少，账上就此有据可查（M4 loop 用）。
func (s *Session) RecordAssistantFinal(final AssistantFinalData, replaces []int) (Event, error) {
	return s.append(KindAssistantFinal, final, replaces, false)
}

// RecordToolCall 记模型要调工具（M3 tools 用）。
func (s *Session) RecordToolCall(call ToolCallData) (Event, error) {
	return s.append(KindToolCall, call, nil, false)
}

// RecordToolStart 记工具真的开始执行了——副作用窗口从这里开始（M3 tools 用）。
func (s *Session) RecordToolStart(callID string) (Event, error) {
	return s.append(KindToolStart, ToolStartData{CallID: callID}, nil, false)
}

// RecordToolResult 记工具的回话：成功/失败/跳过，必须是终态（M3 tools 用）。
func (s *Session) RecordToolResult(result ToolResultData) (Event, error) {
	return s.append(KindToolResult, result, nil, false)
}

// RecordTurnStart / RecordTurnEnd 记一轮的边界（M4 loop 用）。
func (s *Session) RecordTurnStart() (Event, error) {
	return s.append(KindTurnStart, struct{}{}, nil, false)
}

func (s *Session) RecordTurnEnd() (Event, error) {
	return s.append(KindTurnEnd, struct{}{}, nil, false)
}

// RecordStepStart / RecordStepEnd 记一步的边界；一步结束即一次模型回复收口（M4 loop 用）。
func (s *Session) RecordStepStart() (Event, error) {
	return s.append(KindStepStart, struct{}{}, nil, false)
}

func (s *Session) RecordStepEnd() (Event, error) {
	return s.append(KindStepEnd, struct{}{}, nil, false)
}

// RecordDeliver 记一份进收件箱的投递：只入账，模型还看不见（M4 loop 用）。
func (s *Session) RecordDeliver(id string, text string, target string) (Event, error) {
	return s.append(KindDeliver, DeliverData{ID: id, Text: text, Target: target}, nil, false)
}

// RecordClaim 记一次领出：这时消息才变成用户的话（M4 loop 用）。
func (s *Session) RecordClaim(id string) (Event, error) {
	return s.append(KindClaim, ClaimData{ID: id}, nil, false)
}

// RecordSnapshot 存档当年发给模型的那份完整请求（M4 loop 用）。
func (s *Session) RecordSnapshot(requestJSON []byte) (Event, error) {
	return s.append(KindSnapshot, SnapshotData{Request: requestJSON}, nil, false)
}

// RecordSummary 记压缩摘要：声明取代哪些旧账，模型以后只看这份（压缩插件用）。
// data 里顺手记下哪个模型写的摘要、花了多少 token——将来排查"这摘要谁写的"有答案。
func (s *Session) RecordSummary(data SummaryData, replaces []int) (Event, error) {
	return s.append(KindSummary, data, replaces, false)
}

// Flush 把这本账攒着没写的部分立刻全部写完并到硬盘。
// 危险操作（调模型、执行工具）之前必须调：万一断电，账上至少知道刚才断在哪。
func (s *Session) Flush() error {
	return s.journal.Flush(s.header.ID)
}

// ---- 记账全流程：拿编号 -> 验内容 -> 写盘 -> 写内存 -> 通知 ----

// append 记一笔并广播。广播窗口内的同步调用被拒（ErrInBroadcast）。
// 失败时内存不加、编号不烧（下一笔自然复用）、不广播。
func (s *Session) append(kind string, data any, replaces []int, skip bool) (Event, error) {
	if s.inBroadcast() {
		return Event{}, ErrInBroadcast
	}

	event, err := s.appendLocked(kind, data, replaces, skip)
	if err != nil {
		return Event{}, err
	}

	s.broadcastAndDrain(event)
	return event, nil
}

// appendLocked 纯记账，不广播；补记走这里（不需要广播拒绝检查）。
func (s *Session) appendLocked(kind string, data any, replaces []int, skip bool) (Event, error) {
	s.mu.Lock()
	raw, err := json.Marshal(data)
	if err != nil {
		s.mu.Unlock()
		return Event{}, fmt.Errorf("第 %d 笔（%s）内容记不成 JSON：%w", len(s.events)+1, kind, err)
	}
	for _, replaced := range replaces {
		if replaced < 1 || replaced > len(s.events) {
			s.mu.Unlock()
			return Event{}, fmt.Errorf("种类 %s 要取代第 %d 笔，但账上没有这笔", kind, replaced)
		}
	}

	event := Event{
		Kind:          kind,
		Seq:           len(s.events) + 1,
		Time:          now(),
		Data:          raw,
		Replaces:      replaces,
		SkipIfUnknown: skip,
	}

	err = s.journal.Append(s.header.ID, event, isDurable(kind))
	if err != nil {
		s.mu.Unlock()
		return Event{}, fmt.Errorf("第 %d 笔（%s）落盘失败，整笔记失败：%w", event.Seq, kind, err)
	}

	s.events = append(s.events, cloneEvent(event))
	s.history.note(cloneEvent(event), s.events)
	s.mu.Unlock()

	return event, nil
}

// broadcastAndDrain 广播新账，然后逐笔消化广播期间投递的补记。
// 补记的广播在本循环里就地完成、新投递继续排队——迭代不递归，
// 队列再怎么自激也只是排队，不会一层层套栈。
func (s *Session) broadcastAndDrain(event Event) {
	s.broadcastOne(event)

	for {
		s.queueMu.Lock()
		if len(s.queue) == 0 {
			s.queueMu.Unlock()
			return
		}
		job := s.queue[0]
		s.queue = s.queue[1:]
		s.queueMu.Unlock()

		skip := false
		if format, known := s.formats.lookup(job.kind); known {
			skip = format.skipIfUnknown
		}
		appended, err := s.appendLocked(job.kind, job.data, job.replaces, skip)
		if err != nil {
			// 补记没有等待者，失败只能留痕。
			log.Printf("session: 补记 %s 失败：%v", job.kind, err)
			continue
		}
		s.broadcastOne(appended)
	}
}

// broadcastOne 抬起广播标记、喊一嗓子（带账本号）、放下标记；
// 标记期间到达的同步记账被拒，异步投递进队。
func (s *Session) broadcastOne(event Event) {
	s.queueMu.Lock()
	s.depth++
	s.queueMu.Unlock()

	s.broadcaster.Broadcast(EventAppended, Appended{SessionID: s.header.ID, Event: cloneEvent(event)})

	s.queueMu.Lock()
	s.depth--
	s.queueMu.Unlock()
}

// inBroadcast 当前是否处于广播（或补记）窗口内。
func (s *Session) inBroadcast() bool {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()

	return s.depth > 0
}
