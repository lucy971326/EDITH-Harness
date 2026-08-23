package session

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Journal 是落盘器：把账一笔一笔写进某个地方，读回来重放。
// 规矩：要么整条，要么没有——失败后介质上不许留半条。
// 真文件版（一行一条 JSON、fsync、烂尾修复）在 M6；本文件给内存版供单测。
type Journal interface {
	// Create 开一本新账（写封面）；账本号已存在返回 error。
	Create(id string, header Header) error
	// Append 落一笔。durable=true 时成功返回前这笔必须真到硬盘；
	// false 允许攒批（只对流式素材用）。失败后介质不得留半条。
	Append(id string, event Event, durable bool) error
	// Flush 把攒着没写的账立刻全部写完并到硬盘。
	// 危险操作（调模型、执行工具）之前必须调它：万一断电，至少账上知道刚才断在哪。
	Flush(id string) error
	// ReadAll 读回整本账：封面 + 全部笔。
	ReadAll(id string) (Header, []Event, error)
}

// MemoryJournal 是内存假实现：给单测和不落盘的场景。
type MemoryJournal struct {
	mu    sync.Mutex
	books map[string][]Event
}

// NewMemoryJournal 建一个空的内存落盘器。
func NewMemoryJournal() *MemoryJournal {
	return &MemoryJournal{books: make(map[string][]Event)}
}

func (j *MemoryJournal) Create(id string, header Header) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	_, exists := j.books[id]
	if exists {
		return fmt.Errorf("账本 %s 已存在", id)
	}
	j.books[id] = []Event{}
	return nil
}

func (j *MemoryJournal) Append(id string, event Event, durable bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	book, exists := j.books[id]
	if !exists {
		return fmt.Errorf("账本 %s 不存在，先 Create", id)
	}
	j.books[id] = append(book, cloneEvent(event))
	return nil
}

// Flush 对内存版是空操作：账本来就全在手上，没有攒的概念。
// 真文件版在这兑现"立刻全部写完并到硬盘"的承诺。
func (j *MemoryJournal) Flush(id string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	_, exists := j.books[id]
	if !exists {
		return fmt.Errorf("账本 %s 不存在", id)
	}
	return nil
}

func (j *MemoryJournal) ReadAll(id string) (Header, []Event, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	book, exists := j.books[id]
	if !exists {
		return Header{}, nil, fmt.Errorf("账本 %s 不存在", id)
	}
	events := make([]Event, len(book))
	for i, event := range book {
		events[i] = cloneEvent(event)
	}
	header := Header{FormatVersion: 1, ID: id}
	return header, events, nil
}

// cloneEvent 拷一份事件，连 Data 和 Replaces 一起，防止内外共享可改的切片。
func cloneEvent(e Event) Event {
	copied := e
	if e.Data != nil {
		copied.Data = append(json.RawMessage(nil), e.Data...)
	}
	if e.Replaces != nil {
		copied.Replaces = append([]int(nil), e.Replaces...)
	}
	return copied
}
