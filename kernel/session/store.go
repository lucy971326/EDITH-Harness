package session

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"harness/kernel/persist"
)

// Store 是表上那份对话账。live 里是打开过的 Session。
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
	_, err := s.persist.Load(id)
	if err == nil {
		return nil, fmt.Errorf("session: %q already exists", id)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("session: create %q: %w", id, err)
	}
	// 空账没有文件；只有第一次 Append 才会落盘。
	sess := &Session{id: id, disk: s.persist, nodes: make(map[string]persist.Node)}
	s.live[id] = sess
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
	tree, err := s.persist.Load(id)
	if err != nil {
		return nil, fmt.Errorf("session: get %q: %w", id, err)
	}
	sess := &Session{id: id, disk: s.persist, nodes: make(map[string]persist.Node)}
	for _, node := range tree.Nodes {
		sess.nodes[node.ID] = node
		sess.head = node.ID
	}
	s.live[id] = sess
	return sess, nil
}

// List 列出持久化服务知道的账本。
func (s *Store) List() ([]persist.Meta, error) {
	return s.persist.List()
}
