package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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
	next  uint64
}

// Append 在当前光标下面写入一个完整节点。
func (s *Session) Append(m Message) (Entry, error) {
	err := checkMessage(m)
	if err != nil {
		return Entry{}, err
	}
	body, err := json.Marshal(m)
	if err != nil {
		return Entry{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := newNodeID()
	if err != nil {
		return Entry{}, err
	}
	node := persist.Node{ID: id, Parent: s.head, Seq: s.next + 1, Body: body}
	err = s.disk.Add(s.id, node)
	if err != nil {
		return Entry{}, fmt.Errorf("session: append: %w", err)
	}
	s.nodes[id] = node
	s.head = id
	s.next = node.Seq
	// 标题只是消息成功落账后的派生展示；不能让它的失败把已写入的消息变成未通知。
	_ = s.renameFirstUserMessage(m)
	return Entry{ID: node.ID, ParentID: node.Parent, Seq: node.Seq, Message: m}, nil
}

func (s *Session) renameFirstUserMessage(message Message) error {
	if message.Role != RoleUser {
		return nil
	}
	title := titleFromMessage(message)
	if title == "" {
		return nil
	}
	meta, err := s.disk.LoadMeta(s.id)
	if err != nil {
		return fmt.Errorf("session: load metadata: %w", err)
	}
	if meta.Title != "新对话" {
		return nil
	}
	meta.Title = title
	if err := s.disk.SaveMeta(meta); err != nil {
		return fmt.Errorf("session: name first message: %w", err)
	}
	return nil
}

func titleFromMessage(message Message) string {
	for _, block := range message.Blocks {
		if block.Kind != "text" {
			continue
		}
		title := strings.Join(strings.Fields(block.Text), " ")
		runes := []rune(title)
		if len(runes) > 48 {
			return string(runes[:48]) + "…"
		}
		return title
	}
	for _, block := range message.Blocks {
		if block.Kind == "image" {
			return "图片"
		}
	}
	return ""
}

// History 沿当前分叉回到根，再按对话顺序返回发给模型的有效历史。
// 从当前分支最近一次摘要开始：摘要收成普通文本，其后消息原样保留。不按模型裁切图。
func (s *Session) History() []Message {
	entries := s.Entries()
	start := 0
	for index, entry := range entries {
		if isSummary(entry.Message) {
			start = index
		}
	}
	out := make([]Message, 0, len(entries)-start)
	for index := start; index < len(entries); index++ {
		message := entries[index].Message
		if index == start && isSummary(message) {
			out = append(out, projectSummary(message))
			continue
		}
		out = append(out, message)
	}
	return out
}

func isSummary(message Message) bool {
	for _, block := range message.Blocks {
		if block.Kind == "summary" {
			return true
		}
	}
	return false
}

func projectSummary(message Message) Message {
	text := ""
	for _, block := range message.Blocks {
		if block.Kind == "summary" || block.Kind == "text" {
			text += block.Text
		}
	}
	return Message{
		RunID:  message.RunID,
		Role:   RoleAssistant,
		Blocks: []Block{{Kind: "text", Text: text}},
	}
}

// Entries 沿当前分叉回到根，再按对话顺序返回已落账消息和节点身份。
func (s *Session) Entries() []Entry {
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

	out := make([]Entry, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		node := s.nodes[ids[i]]
		var message Message
		err := json.Unmarshal(node.Body, &message)
		if err != nil {
			continue
		}
		out = append(out, Entry{ID: node.ID, ParentID: node.Parent, Seq: node.Seq, Message: message})
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
	if m.Role == RoleCollaboration {
		if m.MessageID == "" || m.SourceSessionID == "" {
			return fmt.Errorf("session: collaboration requires message and source identity")
		}
		for _, block := range m.Blocks {
			if block.Kind != "text" {
				return fmt.Errorf("session: collaboration only accepts text")
			}
		}
	}
	if m.Role == RoleTool && (len(m.Blocks) != 1 || m.Blocks[0].Kind != "tool-result") {
		return fmt.Errorf("session: tool message needs exactly one tool-result block")
	}
	for _, b := range m.Blocks {
		switch b.Kind {
		case "summary":
			if m.Role != RoleAssistant {
				return fmt.Errorf("session: summary block needs assistant role")
			}
			if strings.TrimSpace(b.Text) == "" {
				return fmt.Errorf("session: summary block needs text")
			}
		case "image":
			if b.Media == nil || b.Media.MIME == "" || b.Media.Data == "" {
				return fmt.Errorf("session: image block needs mime and data")
			}
		case "tool-result":
			if m.Role != RoleTool {
				return fmt.Errorf("session: tool-result block needs tool role")
			}
			if b.Result == nil || b.Result.ID == "" || b.Result.Name == "" {
				return fmt.Errorf("session: tool-result block needs id and name")
			}
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
