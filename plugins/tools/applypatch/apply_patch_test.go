package applypatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"harness/kernel/machine"
	"harness/kernel/tools"
)

type memoryMachine struct {
	files         map[string][]byte
	failWritePath string
	afterWrite    func()
}

func (m *memoryMachine) HomeDir() (string, error) { return "/home/test", nil }
func (m *memoryMachine) ReadDir(string) ([]machine.DirEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *memoryMachine) ResolvePath(workspace, path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	return workspace + "/" + path
}
func (m *memoryMachine) ReadFile(path string) ([]byte, error) {
	data, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}
func (m *memoryMachine) ReadFileVersion(path string, _ int64) (machine.FileContent, error) {
	data, err := m.ReadFile(path)
	if err != nil {
		return machine.FileContent{}, err
	}
	return machine.FileContent{Data: data, Hash: testHash(data)}, nil
}
func (m *memoryMachine) Metadata(string) (machine.FileMetadata, error) {
	return machine.FileMetadata{}, errors.New("not implemented")
}
func (m *memoryMachine) Watch(string) (machine.FileWatch, error) {
	return nil, errors.New("not implemented")
}
func (m *memoryMachine) WriteFile(path string, data []byte) error {
	if path == m.failWritePath {
		return errors.New("write failed")
	}
	m.files[path] = append([]byte(nil), data...)
	if m.afterWrite != nil {
		m.afterWrite()
	}
	return nil
}
func (m *memoryMachine) WriteFileIfUnchanged(path string, data []byte, expectedHash string) (string, error) {
	current, exists := m.files[path]
	if expectedHash == machine.AbsentFileHash {
		if exists {
			return "", machine.ErrFileConflict
		}
	} else if !exists || testHash(current) != expectedHash {
		return "", machine.ErrFileConflict
	}
	err := m.WriteFile(path, data)
	if err != nil {
		return "", err
	}
	return testHash(data), nil
}
func (m *memoryMachine) RemoveFileIfUnchanged(path string, expectedHash string) error {
	current, exists := m.files[path]
	if !exists || testHash(current) != expectedHash {
		return machine.ErrFileConflict
	}
	delete(m.files, path)
	return nil
}

func testHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type changingMachine struct {
	*memoryMachine
	changed bool
}

func (m *changingMachine) WriteFileIfUnchanged(path string, data []byte, expectedHash string) (string, error) {
	if !m.changed {
		m.files[path] = []byte("changed elsewhere\n")
		m.changed = true
	}
	return m.memoryMachine.WriteFileIfUnchanged(path, data, expectedHash)
}

func TestApplyPatchValidatesEverythingBeforeWriting(t *testing.T) {
	m := &memoryMachine{files: map[string][]byte{"/work/first.txt": []byte("before\n"), "/work/second.txt": []byte("actual\n")}}
	patch := `*** Begin Patch
*** Update File: first.txt
@@
-before
+after
*** Update File: second.txt
@@
-expected
+changed
*** End Patch`

	delta, _, err := applyPatch(context.Background(), m, "/work", patch)
	if err == nil || !strings.Contains(err.Error(), "failed to find expected lines") {
		t.Fatalf("error = %v", err)
	}
	if len(delta.Changes) != 0 || string(m.files["/work/first.txt"]) != "before\n" {
		t.Fatalf("partial write before validation: delta=%#v files=%#v", delta, m.files)
	}
}

func TestApplyPatchRejectsConcurrentChange(t *testing.T) {
	m := &changingMachine{memoryMachine: &memoryMachine{files: map[string][]byte{"/work/text.txt": []byte("before\n")}}}
	patch := `*** Begin Patch
*** Update File: text.txt
@@
-before
+after
*** End Patch`

	delta, _, err := applyPatch(context.Background(), m, "/work", patch)
	if !errors.Is(err, machine.ErrFileConflict) || len(delta.Changes) != 0 {
		t.Fatalf("delta = %#v, error = %v", delta, err)
	}
	if got := string(m.files["/work/text.txt"]); got != "changed elsewhere\n" {
		t.Fatalf("concurrent content was overwritten: %q", got)
	}
}

func TestApplyPatchKeepsMarkerLikeContextInCurrentFile(t *testing.T) {
	m := &memoryMachine{files: map[string][]byte{
		"/work/doc.md": []byte("before\n*** Update File: literal\nold\n"),
	}}
	patch := `*** Begin Patch
*** Update File: doc.md
@@
 before
 *** Update File: literal
-old
+new
*** End Patch`

	_, _, err := applyPatch(context.Background(), m, "/work", patch)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(m.files["/work/doc.md"]); got != "before\n*** Update File: literal\nnew\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyPatchPreservesLineEndingsAndMatchesUnicodePunctuation(t *testing.T) {
	m := &memoryMachine{files: map[string][]byte{
		"/work/text.txt": []byte("start\r\nsmart — quote “x”\r\nend\r\n"),
	}}
	patch := `*** Begin Patch
*** Update File: text.txt
@@
-smart - quote "x"
+changed
*** End Patch`

	_, _, err := applyPatch(context.Background(), m, "/work", patch)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(m.files["/work/text.txt"]); got != "start\r\nchanged\r\nend\r\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyPatchSupportsOrderedChunksAndEndOfFile(t *testing.T) {
	m := &memoryMachine{files: map[string][]byte{
		"/work/text.txt": []byte("header\nfirst\nmiddle\nlast\n"),
	}}
	patch := `*** Begin Patch
*** Update File: text.txt
@@ header
-first
+FIRST
@@
-middle
+MIDDLE
 last
*** End of File
*** End Patch`

	_, _, err := applyPatch(context.Background(), m, "/work", patch)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(m.files["/work/text.txt"]); got != "header\nFIRST\nMIDDLE\nlast\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyPatchReportsCommittedPrefix(t *testing.T) {
	m := &memoryMachine{files: make(map[string][]byte), failWritePath: "/work/second.txt"}
	patch := `*** Begin Patch
*** Add File: first.txt
+first
*** Add File: second.txt
+second
*** End Patch`

	delta, _, err := applyPatch(context.Background(), m, "/work", patch)
	if err == nil || len(delta.Changes) != 1 || delta.Changes[0].Path != "/work/first.txt" || delta.Exact {
		t.Fatalf("delta = %#v, error = %v", delta, err)
	}
	if string(m.files["/work/first.txt"]) != "first\n" {
		t.Fatalf("first.txt = %q", m.files["/work/first.txt"])
	}
}

func TestApplyPatchToolReturnsAppliedFileDelta(t *testing.T) {
	m := &memoryMachine{files: map[string][]byte{"/work/text.txt": []byte("before\n")}}
	registry := tools.NewRegistry()
	if err := registry.Register(newTool(m)); err != nil {
		t.Fatal(err)
	}
	result, err := registry.Call(t.Context(), tools.Call{
		Name:      "apply_patch",
		Workspace: "/work",
		Allow:     []string{"apply_patch"},
		Arguments: []byte(`{"patch":"*** Begin Patch\n*** Update File: text.txt\n@@\n-before\n+after\n*** End Patch"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FileDelta == nil || !result.FileDelta.Exact || len(result.FileDelta.Changes) != 1 {
		t.Fatalf("file delta = %#v", result.FileDelta)
	}
	change := result.FileDelta.Changes[0]
	if change.Path != "/work/text.txt" || change.Operation != tools.FileOperationUpdate || *change.OldContent != "before\n" || *change.NewContent != "after\n" {
		t.Fatalf("change = %#v", change)
	}
}

func TestApplyPatchToolReturnsCommittedDeltaWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &memoryMachine{files: make(map[string][]byte), afterWrite: cancel}
	registry := tools.NewRegistry()
	if err := registry.Register(newTool(m)); err != nil {
		t.Fatal(err)
	}
	result, err := registry.Call(ctx, tools.Call{
		Name:      "apply_patch",
		Workspace: "/work",
		Allow:     []string{"apply_patch"},
		Arguments: []byte(`{"patch":"*** Begin Patch\n*** Add File: first.txt\n+first\n*** Add File: second.txt\n+second\n*** End Patch"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.FileDelta == nil || !result.FileDelta.Exact || len(result.FileDelta.Changes) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestApplyPatchRejectsInvalidUTF8BeforeWriting(t *testing.T) {
	m := &memoryMachine{files: map[string][]byte{
		"/work/bad.txt":  {0xff},
		"/work/good.txt": []byte("before\n"),
	}}
	patch := "*** Begin Patch\n*** Update File: good.txt\n@@\n-before\n+after\n*** Delete File: bad.txt\n*** End Patch"

	delta, _, err := applyPatch(context.Background(), m, "/work", patch)
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("delta = %#v, error = %v", delta, err)
	}
	if got := string(m.files["/work/good.txt"]); got != "before\n" {
		t.Fatalf("good.txt was written before UTF-8 validation: %q", got)
	}
}

func TestParsePatchRejectsMoveAndMalformedInput(t *testing.T) {
	for _, patch := range []string{
		"*** Update File: x\n@@\n-a\n+b",
		"*** Begin Patch\n*** Update File: x\n*** Move to: y\n@@\n-a\n+b\n*** End Patch",
		"*** Begin Patch\n*** Add File: x\nnot-added\n*** End Patch",
	} {
		if _, err := parsePatch(patch); err == nil {
			t.Fatalf("parsePatch(%q) error = nil", patch)
		}
	}
}
