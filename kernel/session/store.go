package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"harness/kernel/persist"
)

// 活对象。表上那份对话账。live 里是打开过的 Session。
type Store struct {
	persist persist.Persistence
	mu      sync.Mutex
	live    map[string]*Session
}

// NewStore 造一个空 Store。账本在 Persistence 上。
func NewStore(p persist.Persistence) *Store {
	return &Store{persist: p, live: make(map[string]*Session)}
}

// Create 创建一本空账。id 必须尚未被使用。
func (s *Store) Create(id string) (*Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session: empty id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.live[id]; ok {
		return nil, fmt.Errorf("session: %q already exists", id)
	}
	_, err := s.persist.LoadMeta(id)
	if err == nil {
		return nil, fmt.Errorf("session: %q already exists", id)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("session: create %q: %w", id, err)
	}
	err = s.persist.SaveMeta(persist.Meta{ID: id, Title: "新对话", CreatedAt: time.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("session: create %q: %w", id, err)
	}

	// 空账没有文件；元数据文件已经表明会话存在。
	sess := &Session{id: id, disk: s.persist, nodes: make(map[string]persist.Node)}
	s.live[id] = sess
	return sess, nil
}

// Fork 将来源会话当前分叉中截至目标节点的历史复制为一场独立会话。
func (s *Store) Fork(sourceID, destinationID, throughEntryID, title string) (*Session, error) {
	if sourceID == "" || destinationID == "" || throughEntryID == "" {
		return nil, fmt.Errorf("session: fork has empty required field")
	}
	if sourceID == destinationID {
		return nil, fmt.Errorf("session: fork source and destination are the same")
	}
	if title == "" {
		return nil, fmt.Errorf("session: fork has empty title")
	}
	source, err := s.Get(sourceID)
	if err != nil {
		return nil, fmt.Errorf("session: fork source: %w", err)
	}
	entries := source.Entries()
	through := -1
	for index, entry := range entries {
		if entry.ID == throughEntryID {
			through = index
			break
		}
	}
	if through < 0 {
		return nil, fmt.Errorf("session: fork target %q is not in current branch", throughEntryID)
	}

	nodes := make([]persist.Node, 0, through+1)
	cloned := make(map[string]persist.Node, through+1)
	parent := ""
	for index, entry := range entries[:through+1] {
		body, err := json.Marshal(entry.Message)
		if err != nil {
			return nil, fmt.Errorf("session: fork encode entry %q: %w", entry.ID, err)
		}
		id, err := newNodeID()
		if err != nil {
			return nil, err
		}
		node := persist.Node{ID: id, Parent: parent, Seq: uint64(index + 1), Body: body}
		nodes = append(nodes, node)
		cloned[id] = node
		parent = id
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.live[destinationID]; exists {
		return nil, fmt.Errorf("session: %q already exists", destinationID)
	}
	_, err = s.persist.LoadMeta(destinationID)
	if err == nil {
		return nil, fmt.Errorf("session: %q already exists", destinationID)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("session: fork destination %q: %w", destinationID, err)
	}
	err = s.persist.Save(destinationID, &persist.Tree{ID: destinationID, Nodes: nodes})
	if err != nil {
		return nil, fmt.Errorf("session: fork save ledger: %w", err)
	}
	err = s.persist.SaveMeta(persist.Meta{ID: destinationID, Title: title, CreatedAt: time.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("session: fork save metadata: %w", err)
	}
	sess := &Session{id: destinationID, disk: s.persist, nodes: cloned, head: parent, next: uint64(len(nodes))}
	s.live[destinationID] = sess
	return sess, nil
}

// Get 返回同一个 id 对应的同一个活对象。
func (s *Store) Get(id string) (*Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session: empty id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.live[id]; ok {
		return sess, nil
	}
	_, err := s.persist.LoadMeta(id)
	if err != nil {
		return nil, fmt.Errorf("session: get %q: %w", id, err)
	}
	sess := &Session{id: id, disk: s.persist, nodes: make(map[string]persist.Node)}
	tree, err := s.persist.Load(id)
	if errors.Is(err, os.ErrNotExist) {
		s.live[id] = sess
		return sess, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session: get %q: %w", id, err)
	}
	for _, node := range tree.Nodes {
		if node.Seq == 0 {
			return nil, fmt.Errorf("session: ledger %q uses unsupported old format; please clear old data", id)
		}
		sess.nodes[node.ID] = node
		sess.head = node.ID
		if node.Seq > sess.next {
			sess.next = node.Seq
		}
	}
	s.live[id] = sess
	return sess, nil
}

// List 列出持久化服务知道的会话。
func (s *Store) List() ([]SessionMeta, error) {
	metas, err := s.persist.List()
	if err != nil {
		return nil, err
	}
	out := make([]SessionMeta, 0, len(metas))
	for _, meta := range metas {
		out = append(out, SessionMeta{ID: meta.ID, Title: meta.Title, CreatedAt: meta.CreatedAt})
	}
	return out, nil
}

// Rename 修改一本会话的显示标题。
func (s *Store) Rename(id string, title string) error {
	if id == "" {
		return fmt.Errorf("session: empty id")
	}
	if title == "" {
		return fmt.Errorf("session: empty title")
	}
	meta, err := s.persist.LoadMeta(id)
	if err != nil {
		return fmt.Errorf("session: rename %q: %w", id, err)
	}
	meta.Title = title
	err = s.persist.SaveMeta(meta)
	if err != nil {
		return fmt.Errorf("session: rename %q: %w", id, err)
	}
	return nil
}

// DiscardEmpty 删除刚创建、尚未写入账本的会话。
func (s *Store) DiscardEmpty(id string) error {
	if id == "" {
		return fmt.Errorf("session: empty id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.live[id]
	if !ok {
		return fmt.Errorf("session: %q is not live", id)
	}
	sess.mu.Lock()
	empty := sess.head == ""
	sess.mu.Unlock()
	if !empty {
		return fmt.Errorf("session: %q has messages", id)
	}
	err := s.persist.DeleteMeta(id)
	if err != nil {
		return fmt.Errorf("session: discard %q: %w", id, err)
	}
	delete(s.live, id)
	return nil
}
