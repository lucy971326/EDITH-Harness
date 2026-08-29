package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"harness/kernel/persist"
)

// 活对象。一本对话账。nodes 和磁盘上的节点保持同一份追加顺序。
type Session struct {
	mu    sync.Mutex
	id    string
	disk  persist.Persistence
	nodes map[string]persist.Node
	head  string
}

// Append 在当前光标下面写入一个完整节点。
func (s *Session) Append(m Message) error {
	err := checkMessage(m)
	if err != nil {
		return err
	}
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := newNodeID()
	if err != nil {
		return err
	}
	node := persist.Node{ID: id, Parent: s.head, Body: body}
	err = s.disk.Add(s.id, node)
	if err != nil {
		return fmt.Errorf("session: append: %w", err)
	}
	s.nodes[id] = node
	s.head = id
	return nil
}

// History 沿当前分叉回到根，再按对话顺序返回。不按模型裁切图。
func (s *Session) History() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0)
	for id := s.head; id != ""; {
		node, ok := s.nodes[id]
		if !ok {
			break
		}
		ids = append(ids, id)
		id = node.Parent
	}

	out := make([]Message, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		var message Message
		err := json.Unmarshal(s.nodes[ids[i]].Body, &message)
		if err != nil {
			continue
		}
		out = append(out, message)
	}
	return out
}

// Branch 只移动光标，不修改账本。
func (s *Session) Branch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id != "" {
		if _, ok := s.nodes[id]; !ok {
			return fmt.Errorf("session: node %q not found", id)
		}
	}
	s.head = id
	return nil
}

// Head 返回当前光标所在节点。
func (s *Session) Head() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.head
}

func checkMessage(m Message) error {
	for _, b := range m.Blocks {
		if b.Kind != "image" {
			continue
		}
		if b.Media == nil || b.Media.MIME == "" || b.Media.Data == "" {
			return fmt.Errorf("session: image block needs mime and data")
		}
	}
	return nil
}

func newNodeID() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", fmt.Errorf("session: make node id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
