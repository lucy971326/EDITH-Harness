package session

import (
	"fmt"
	"sync"
)

// Store 是账本管家：开新账、按号取账、给插件注册自定义种类。
type Store struct {
	mu          sync.Mutex
	journal     Journal
	broadcaster Broadcaster
	formats     *formatTable
	sessions    map[string]*Session
}

// NewStore 建管家。落盘器和广播口都从外面注入：core.App 天然能当广播口。
func NewStore(journal Journal, broadcaster Broadcaster) *Store {
	return &Store{
		journal:     journal,
		broadcaster: broadcaster,
		formats:     newFormatTable(),
		sessions:    make(map[string]*Session),
	}
}

// RegisterKind 给插件注册自定义事件种类。skipIfUnknown=true 表示
// 插件将来不在了读到这笔就跳过；false 表示整本账拒读（宁严勿猜）。
func (st *Store) RegisterKind(kind string, skipIfUnknown bool) error {
	return st.formats.register(kind, skipIfUnknown)
}

// Create 开一本新账。带 seed 就是重放一份旧账（重建内存态，逐笔写盘，不广播）。
func (st *Store) Create(id string, seed ...Event) (*Session, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	_, exists := st.sessions[id]
	if exists {
		return nil, fmt.Errorf("账本 %s 已经开过了", id)
	}

	header := Header{FormatVersion: 1, ID: id}
	err := st.journal.Create(id, header)
	if err != nil {
		return nil, err
	}

	s := newSession(header, st.journal, st.broadcaster, st.formats)

	err = s.replay(seed)
	if err != nil {
		return nil, fmt.Errorf("账本 %s 的旧账重放失败：%w", id, err)
	}

	st.sessions[id] = s
	return s, nil
}

// Get 按号取一本已开的账。
func (st *Store) Get(id string) (*Session, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	s, exists := st.sessions[id]
	if !exists {
		return nil, fmt.Errorf("账本 %s 没开过", id)
	}
	return s, nil
}

// replay 重放旧账：编号必须从 1 连续，每笔过体检，逐笔写盘进内存，不广播。
func (s *Session) replay(seed []Event) error {
	for i, event := range seed {
		if event.Seq != i+1 {
			return fmt.Errorf("编号断了：第 %d 笔的编号是 %d", i+1, event.Seq)
		}
		for _, replaced := range event.Replaces {
			if replaced < 1 || replaced > i {
				return fmt.Errorf("第 %d 笔要取代第 %d 笔，但重放到这里还没有这笔", event.Seq, replaced)
			}
		}
		err := s.formats.checkRead(event)
		if err != nil {
			return err
		}
		err = s.journal.Append(s.header.ID, event, isDurable(event.Kind))
		if err != nil {
			return err
		}
	}

	if len(seed) > 0 {
		s.mu.Lock()
		s.events = append(s.events, seed...)
		s.history.rebuild(s.events)
		s.mu.Unlock()
	}
	return nil
}
