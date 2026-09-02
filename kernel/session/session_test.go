package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"harness/kernel/host"
	"harness/kernel/persist"
)

func newTestStore(t *testing.T) (*Store, persist.Persistence) {
	t.Helper()
	h := host.NewHost()
	err := h.Install(&persist.Plugin{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	p, err := host.Resolve[persist.Persistence](h, "sessionPersistence")
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(p), p
}

func textMessage(role Role, text string) Message {
	return Message{Role: role, Blocks: []Block{{Kind: "text", Text: text}}}
}

func TestCreateAppendHistory(t *testing.T) {
	store, _ := newTestStore(t)
	s, err := store.Create("chat1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append(textMessage(RoleUser, "hi"))
	if err != nil {
		t.Fatal(err)
	}
	got := s.History()
	if len(got) != 1 || got[0].Blocks[0].Text != "hi" {
		t.Fatalf("history = %+v", got)
	}
	metas, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if metas[0].Title != "hi" {
		t.Fatalf("title = %q", metas[0].Title)
	}
}

func TestFirstUserMessageNamesSessionOnlyOnce(t *testing.T) {
	store, _ := newTestStore(t)
	s, err := store.Create("chat1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append(textMessage(RoleAssistant, "answer"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append(textMessage(RoleUser, " first\nquestion "))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append(textMessage(RoleUser, "second"))
	if err != nil {
		t.Fatal(err)
	}
	metas, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if metas[0].Title != "first question" {
		t.Fatalf("title = %q", metas[0].Title)
	}
}

func TestEmptySessionSurvivesNewStoreAndList(t *testing.T) {
	dir := t.TempDir()
	h := host.NewHost()
	err := h.Install(&persist.Plugin{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	p, err := host.Resolve[persist.Persistence](h, "sessionPersistence")
	if err != nil {
		t.Fatal(err)
	}
	first := NewStore(p)
	_, err = first.Create("empty")
	if err != nil {
		t.Fatal(err)
	}
	again := NewStore(p)
	loaded, err := again.Get("empty")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.History()) != 0 {
		t.Fatalf("history = %#v", loaded.History())
	}
	list, err := again.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "empty" || list[0].Title != "新对话" || list[0].CreatedAt.IsZero() {
		t.Fatalf("list = %#v", list)
	}
}

func TestDiscardEmptyRemovesSessionMeta(t *testing.T) {
	store, _ := newTestStore(t)
	_, err := store.Create("empty")
	if err != nil {
		t.Fatal(err)
	}
	err = store.DiscardEmpty("empty")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Get("empty")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Get() error = %v, want not exist", err)
	}
}

func TestAppendSurvivesNewStore(t *testing.T) {
	dir := t.TempDir()
	h := host.NewHost()
	err := h.Install(&persist.Plugin{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	p, err := host.Resolve[persist.Persistence](h, "sessionPersistence")
	if err != nil {
		t.Fatal(err)
	}
	first := NewStore(p)
	s, err := first.Create("chat1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append(textMessage(RoleUser, "saved"))
	if err != nil {
		t.Fatal(err)
	}

	again := NewStore(p)
	loaded, err := again.Get("chat1")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.History()) != 1 {
		t.Fatal("saved message missing after reload")
	}
}

func TestGetRejectsOldLedgerWithoutSequence(t *testing.T) {
	store, p := newTestStore(t)
	_, err := store.Create("old")
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(textMessage(RoleUser, "old message"))
	if err != nil {
		t.Fatal(err)
	}
	err = p.Add("old", persist.Node{ID: "old-node", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewStore(p).Get("old")
	if err == nil || !strings.Contains(err.Error(), "unsupported old format") {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestBranchKeepsTreeAndSelectsHistory(t *testing.T) {
	store, _ := newTestStore(t)
	s, err := store.Create("chat1")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"one", "two"} {
		_, err = s.Append(textMessage(RoleUser, text))
		if err != nil {
			t.Fatal(err)
		}
	}
	n2 := s.Head()
	err = s.Branch(s.parentOf(t, n2))
	if err != nil {
		t.Fatal(err)
	}
	// 从旧节点重新接一条路，旧路仍然留在树中。
	_, err = s.Append(textMessage(RoleUser, "other"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.History()) != 2 || s.History()[1].Blocks[0].Text != "other" {
		t.Fatalf("branch history = %+v", s.History())
	}
	err = s.Branch(n2)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.History()) != 2 || s.History()[1].Blocks[0].Text != "two" {
		t.Fatalf("old branch lost: %+v", s.History())
	}
}

func TestEntriesKeepPersistentSequenceAcrossBranches(t *testing.T) {
	store, _ := newTestStore(t)
	s, err := store.Create("chat1")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Append(textMessage(RoleUser, "one"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append(textMessage(RoleAssistant, "two"))
	if err != nil {
		t.Fatal(err)
	}
	err = s.Branch(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	third, err := s.Append(textMessage(RoleUser, "other"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Seq != 1 || third.Seq != 3 {
		t.Fatalf("sequences = %d, %d", first.Seq, third.Seq)
	}
	entries := s.Entries()
	if len(entries) != 2 || entries[0].Seq != 1 || entries[1].Seq != 3 {
		t.Fatalf("entries = %#v", entries)
	}
}

func (s *Session) parentOf(t *testing.T, id string) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nodes[id].Parent
}

func TestBranchDoesNotDeleteOldPath(t *testing.T) {
	store, p := newTestStore(t)
	s, err := store.Create("chat1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append(textMessage(RoleUser, "root"))
	if err != nil {
		t.Fatal(err)
	}
	root := s.Head()
	_, err = s.Append(textMessage(RoleUser, "old"))
	if err != nil {
		t.Fatal(err)
	}
	err = s.Branch(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append(textMessage(RoleUser, "new"))
	if err != nil {
		t.Fatal(err)
	}
	tree, err := p.Load("chat1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(tree.Nodes))
	}
}

func TestGetMissingReturnsError(t *testing.T) {
	store, _ := newTestStore(t)
	_, err := store.Get("missing")
	if err == nil {
		t.Fatal("want missing session error")
	}
}

func TestInstallRegistersStore(t *testing.T) {
	h := host.NewHost()
	err := h.Install(&persist.Plugin{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := host.Resolve[*Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	if store == nil {
		t.Fatal("nil store")
	}
}

func TestImageStoresBase64(t *testing.T) {
	store, p := newTestStore(t)
	s, err := store.Create("chat1")
	if err != nil {
		t.Fatal(err)
	}
	message := Message{Role: RoleUser, Blocks: []Block{{Kind: "image", Media: &Media{MIME: "image/png", Data: "iVBORw0KGgo"}}}}
	_, err = s.Append(message)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := p.Load("chat1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(tree.Nodes[0].Body, []byte("iVBORw0KGgo")) {
		t.Fatal("base64 missing from ledger")
	}
	var got Message
	err = json.Unmarshal(tree.Nodes[0].Body, &got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Media.Data != "iVBORw0KGgo" {
		t.Fatalf("data = %q", got.Blocks[0].Media.Data)
	}
}

func TestImageNeedsMIMEAndData(t *testing.T) {
	store, _ := newTestStore(t)
	s, err := store.Create("chat1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append(Message{Role: RoleUser, Blocks: []Block{{Kind: "image"}}})
	if err == nil {
		t.Fatal("want error on empty image")
	}
	_, err = s.Append(Message{Role: RoleUser, Blocks: []Block{{Kind: "image", Media: &Media{MIME: "image/png"}}}})
	if err == nil {
		t.Fatal("want error on missing data")
	}
}

func TestToolResultNeedsOnePairedBlock(t *testing.T) {
	store, _ := newTestStore(t)
	s, err := store.Create("chat1")
	if err != nil {
		t.Fatal(err)
	}
	invalid := Message{Role: RoleTool, Blocks: []Block{{Kind: "text", Text: "result"}}}
	if _, err := s.Append(invalid); err == nil {
		t.Fatal("want malformed tool result error")
	}
	valid := Message{Role: RoleTool, Blocks: []Block{{
		Kind:   "tool-result",
		Result: &ToolResult{ID: "call_1", Name: "read", Content: "ok"},
	}}}
	if _, err := s.Append(valid); err != nil {
		t.Fatal(err)
	}
}
