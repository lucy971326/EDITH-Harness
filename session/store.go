package session

import (
	"fmt"
	"sync"
)

// Store 是账本管家：开新账、打开旧账、按号取账、给插件注册自定义种类。
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

// Create 开一本新账。agentID 和 profileRevision 标明它属于谁；带 seed
// 就是重放一份旧账（重建内存态，逐笔写盘，不广播）。
func (st *Store) Create(id string, agentID string, profileRevision int, seed ...Event) (*Session, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if agentID == "" {
		return nil, fmt.Errorf("账本 %s 没写所属 agent", id)
	}
	if profileRevision < 1 {
		return nil, fmt.Errorf("账本 %s 的 agent 档案版本必须从 1 起", id)
	}

	_, exists := st.sessions[id]
	if exists {
		return nil, fmt.Errorf("账本 %s 已经开过了", id)
	}

	header := Header{FormatVersion: 2, ID: id, AgentID: agentID, ProfileRevision: profileRevision}
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

// Open 从 Journal 读回一份已存在的账，重建内存状态但不重复写旧账。
func (st *Store) Open(id string) (*Session, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	_, exists := st.sessions[id]
	if exists {
		return nil, fmt.Errorf("账本 %s 已经打开", id)
	}
	header, events, err := st.journal.ReadAll(id)
	if err != nil {
		return nil, err
	}
	if header.FormatVersion != 2 {
		return nil, fmt.Errorf("账本 %s 的格式版本是 %d，当前只认识 2；旧账需要迁移后才能打开", id, header.FormatVersion)
	}
	if header.ID != id {
		return nil, fmt.Errorf("要打开账本 %s，读到的却是 %s", id, header.ID)
	}
	if header.AgentID == "" || header.ProfileRevision < 1 {
		return nil, fmt.Errorf("账本 %s 没有完整的 agent 归属信息，需要迁移后才能打开", id)
	}

	s := newSession(header, st.journal, st.broadcaster, st.formats)
	err = s.restore(events)
	if err != nil {
		return nil, fmt.Errorf("账本 %s 的旧账重建失败：%w", id, err)
	}
	st.sessions[id] = s
	return s, nil
}

// Release 先把账本写穿，再从本进程的打开表摘掉；之后可再次 Open。
// 即使写穿失败也摘掉，避免一次失败把后续恢复永久卡住。
func (st *Store) Release(id string) error {
	st.mu.Lock()
	s, exists := st.sessions[id]
	if exists {
		delete(st.sessions, id)
	}
	st.mu.Unlock()
	if !exists {
		return nil
	}
	return s.Flush()
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

// ListHeaders 列出所有已保存账本的封面；不把它们打开到内存。
func (st *Store) ListHeaders() ([]Header, error) {
	return st.journal.ListHeaders()
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

// restore 把 Journal 读回的旧账装进内存；只校验和重建，不再写回 Journal。
func (s *Session) restore(events []Event) error {
	for index, event := range events {
		if event.Seq != index+1 {
			return fmt.Errorf("编号断了：第 %d 笔的编号是 %d", index+1, event.Seq)
		}
		for _, replaced := range event.Replaces {
			if replaced < 1 || replaced > index {
				return fmt.Errorf("第 %d 笔要取代第 %d 笔，但重建到这里还没有这笔", event.Seq, replaced)
			}
		}
		err := s.formats.checkRead(event)
		if err != nil {
			return err
		}
	}

	restored := make([]Event, len(events))
	for index, event := range events {
		restored[index] = cloneEvent(event)
	}
	s.mu.Lock()
	s.events = restored
	s.history.rebuild(s.events)
	s.mu.Unlock()
	return nil
}
