package conversations

import (
	"errors"
	"os"
	"testing"

	"harness/internal/persist"
)

// 写入可能在原子替换成功后报错；仍必须把该意图视为待恢复删除。
func TestSaveDeletionFreezesWhenWriteOutcomeIsUncertain(t *testing.T) {
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFailed := errors.New("directory sync failed")
	intent := deletionIntent{Version: 1, SessionIDs: []string{"target"}}
	target, err := files.Scope("sessions", "target")
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Write("meta.json", []byte(`{"id":"target"}`)); err != nil {
		t.Fatal(err)
	}
	name, pending, err := saveDeletionWithWrite(files, intent, func(dir *persist.Files, name string, body []byte) error {
		if err := dir.Write(name, body); err != nil {
			return err
		}
		return writeFailed
	})
	if !pending || name == "" || !errors.Is(err, writeFailed) {
		t.Fatalf("post-write failure = name %q, pending %t, err %v", name, pending, err)
	}
	if err := RecoverDeletions(files); err != nil {
		t.Fatalf("recover committed intent: %v", err)
	}
	if _, err := target.Read("meta.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted session survived recovery: %v", err)
	}

	name, pending, err = saveDeletionWithWrite(files, intent, func(*persist.Files, string, []byte) error {
		return writeFailed
	})
	if name != "" || pending || !errors.Is(err, writeFailed) {
		t.Fatalf("pre-write failure = name %q, pending %t, err %v", name, pending, err)
	}
	dir, err := deletionFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := dir.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unexpected deletion intents: %v", entries)
	}
}
