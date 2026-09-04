package persist

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"harness/kernel/agents/config"
	"harness/kernel/host"
	"harness/kernel/session/settings"
)

func TestInstall_twoKeysSameStore(t *testing.T) {
	h := host.NewHost()
	err := h.Install(&Plugin{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	p, err := host.Resolve[Persistence](h, "sessionPersistence")
	if err != nil {
		t.Fatal(err)
	}
	s, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		t.Fatal(err)
	}

	jp, ok := p.(*jsonl)
	if !ok {
		t.Fatalf("persistence is %T", p)
	}
	js, ok := s.(*jsonl)
	if !ok {
		t.Fatalf("session settings is %T", s)
	}
	if jp != js {
		t.Fatal("want the same store on both keys")
	}
	agentStore, err := host.Resolve[config.Store](h, "agentStore")
	if err != nil {
		t.Fatal(err)
	}
	ja, ok := agentStore.(*jsonl)
	if !ok || ja != jp {
		t.Fatal("want the same store on the agentStore key")
	}
}

func TestAgentStore_roundTripListAndDelete(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := config.Agent{ID: "coding", Name: "Coding", Kind: "react", Tools: []string{"bash"}}
	if err := s.PutAgent(input); err != nil {
		t.Fatal(err)
	}
	got, err := s.ForAgent("coding")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != input.ID || got.Name != input.Name || got.Kind != input.Kind || !slices.Equal(got.Tools, input.Tools) {
		t.Fatalf("ForAgent() = %#v, want %#v", got, input)
	}
	list, err := s.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "coding" {
		t.Fatalf("ListAgents() = %#v", list)
	}
	if err := s.DeleteAgent("coding"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ForAgent("coding"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ForAgent() error = %v, want not exist", err)
	}
}

func TestAdd_thenLoad(t *testing.T) {
	dir := t.TempDir()
	s, err := openJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}

	err = s.Add("chat1", Node{
		ID:     "n1",
		Parent: "",
		Body:   json.RawMessage(`{"text":"hi"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	tree, err := s.Load("chat1")
	if err != nil {
		t.Fatal(err)
	}
	if tree.ID != "chat1" {
		t.Fatalf("id = %q", tree.ID)
	}
	if len(tree.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(tree.Nodes))
	}
	if tree.Nodes[0].ID != "n1" {
		t.Fatalf("node id = %q", tree.Nodes[0].ID)
	}
}

func TestAdd_survivesReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := openJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Add("chat1", Node{ID: "n1", Body: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}

	again, err := openJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := again.Load("chat1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Nodes) != 1 || tree.Nodes[0].ID != "n1" {
		t.Fatalf("after reopen: %+v", tree.Nodes)
	}
}

func TestAdd_fork(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	err = s.Add("chat1", Node{ID: "root", Body: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Add("chat1", Node{ID: "a", Parent: "root", Body: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Add("chat1", Node{ID: "b", Parent: "root", Body: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}

	tree, err := s.Load("chat1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(tree.Nodes))
	}

	var kids []string
	for _, n := range tree.Nodes {
		if n.Parent == "root" {
			kids = append(kids, n.ID)
		}
	}
	slices.Sort(kids)
	if !slices.Equal(kids, []string{"a", "b"}) {
		t.Fatalf("kids = %v, want [a b]", kids)
	}
}

func TestList(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = s.SaveMeta(Meta{ID: "one", Title: "One", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	err = s.SaveMeta(Meta{ID: "two", Title: "Two", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range list {
		ids = append(ids, m.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"one", "two"}) {
		t.Fatalf("list = %v", ids)
	}
}

func TestMeta_roundTripWithoutLedger(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := Meta{ID: "empty", Title: "新对话", CreatedAt: time.Now().UTC().Round(0)}
	err = s.SaveMeta(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadMeta("empty")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("meta = %#v, want %#v", got, want)
	}
	_, err = s.Load("empty")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ledger error = %v, want not exist", err)
	}
}

func TestListRejectsMetaWhoseIDDoesNotMatchFilename(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"id":"other","title":"新对话","createdAt":"2026-09-02T00:00:00Z"}`)
	err = s.ensureSessionDir("expected")
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(s.metaFile("expected"), body, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.List()
	if err == nil || !strings.Contains(err.Error(), `has id "other"`) {
		t.Fatalf("List() error = %v", err)
	}
}

func TestSessionSettings_putFor(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	in := settings.SessionSettings{
		AgentID:         "default",
		Model:           "deepseek-v4",
		ReasoningEffort: "high",
		Workspace:       "/workspace",
	}
	err = s.Put("chat1", in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.For("chat1")
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Fatalf("got %+v", got)
	}
	if _, err := os.Stat(s.sessionSettingsFile("chat1")); err != nil {
		t.Fatalf("session settings file: %v", err)
	}
}

func TestSessionSettings_UsesAgent(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("chat1", settings.SessionSettings{AgentID: "coding"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("chat2", settings.SessionSettings{AgentID: "default"}); err != nil {
		t.Fatal(err)
	}
	used, err := s.UsesAgent("coding")
	if err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Fatal("UsesAgent(coding) = false")
	}
	used, err = s.UsesAgent("missing")
	if err != nil {
		t.Fatal(err)
	}
	if used {
		t.Fatal("UsesAgent(missing) = true")
	}
}

func TestSessionSettings_doesNotReadOldSetupFile(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(s.dir, "chat1.setup.json"), []byte(`{"model":"old"}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.For("chat1")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("For() error = %v, want not exist", err)
	}
}

func TestBadID(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	bads := []string{"", ".", "..", "a/b", `a\b`, "../x"}
	for _, id := range bads {
		err := s.Add(id, Node{ID: "n", Body: json.RawMessage(`{}`)})
		if err == nil {
			t.Fatalf("Add(%q): want error", id)
		}
		err = s.Put(id, settings.SessionSettings{})
		if err == nil {
			t.Fatalf("Put(%q): want error", id)
		}
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("wrote files for bad ids: %v", names(entries))
	}
}

func TestSave_roundTrip(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := &Tree{
		ID: "chat1",
		Nodes: []Node{
			{ID: "n1", Body: json.RawMessage(`{"t":1}`)},
			{ID: "n2", Parent: "n1", Body: json.RawMessage(`{"t":2}`)},
		},
	}
	err = s.Save("chat1", want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("chat1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("nodes = %d", len(got.Nodes))
	}
	if got.Nodes[1].Parent != "n1" {
		t.Fatalf("parent = %q", got.Nodes[1].Parent)
	}
}

func TestAdd_emptyNodeID(t *testing.T) {
	s, err := openJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = s.Add("chat1", Node{Body: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatal("want error on empty node id")
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestTreeFileName(t *testing.T) {
	dir := t.TempDir()
	s, err := openJSONL(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Add("chat1", Node{ID: "n", Body: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(dir, "sessions", "chat1", "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
}
