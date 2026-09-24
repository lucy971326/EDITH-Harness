package machinelocal

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"harness/internal/machine"
)

func TestLocal_versionedWritesDetectConflicts(t *testing.T) {
	m := newTestLocal(t)
	path := filepath.Join(t.TempDir(), "版本.txt")
	if err := m.WriteFile(path, []byte("before")); err != nil {
		t.Fatal(err)
	}
	read, err := m.ReadFileVersion(path, 6)
	if err != nil || len(read.Hash) != 64 {
		t.Fatalf("read = %#v, %v", read, err)
	}

	errorsByWriter := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	for _, content := range []string{"first!", "second"} {
		go func() {
			start.Wait()
			_, writeErr := m.WriteFileIfUnchanged(path, []byte(content), read.Hash)
			errorsByWriter <- writeErr
		}()
	}
	start.Done()

	var succeeded, conflicted int
	for range 2 {
		err = <-errorsByWriter
		if err == nil {
			succeeded++
		} else if errors.Is(err, machine.ErrFileConflict) {
			conflicted++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded=%d conflicted=%d", succeeded, conflicted)
	}
	if _, err = m.ReadFileVersion(path, 5); !errors.Is(err, machine.ErrFileTooLarge) {
		t.Fatalf("limited read error = %v", err)
	}

	newPath := filepath.Join(t.TempDir(), "nested", "new.txt")
	_, err = m.WriteFileIfUnchanged(newPath, []byte("new"), machine.AbsentFileHash)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.WriteFileIfUnchanged(newPath, []byte("overwrite"), machine.AbsentFileHash)
	if !errors.Is(err, machine.ErrFileConflict) {
		t.Fatalf("second absent-file write error = %v", err)
	}
}

func TestLocal_removeFileRejectsDirectoriesAndConcurrentChanges(t *testing.T) {
	m := newTestLocal(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := m.WriteFile(path, []byte("note")); err != nil {
		t.Fatal(err)
	}
	read, err := m.ReadFileVersion(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile(path, []byte("changed")); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveFileIfUnchanged(path, read.Hash); !errors.Is(err, machine.ErrFileConflict) {
		t.Fatalf("RemoveFileIfUnchanged(changed file) error = %v", err)
	}
	read, err = m.ReadFileVersion(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveFileIfUnchanged(path, read.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadFile(path); err == nil {
		t.Fatal("removed file is still readable")
	}
	if err := m.RemoveFileIfUnchanged(dir, "unused"); !errors.Is(err, machine.ErrNotRegularFile) {
		t.Fatalf("RemoveFileIfUnchanged(directory) error = %v", err)
	}
}
