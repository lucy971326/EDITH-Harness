package tools_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"harness/kernel/approvals"
	"harness/kernel/machine"
	"harness/kernel/permissions"
	kerneltools "harness/kernel/tools"
	"harness/plugins/tools/applypatch"
)

type fakeMachine struct {
	files map[string][]byte
}

func (m *fakeMachine) HomeDir() (string, error) { return "/home/test", nil }

func (m *fakeMachine) ReadDir(string) ([]machine.DirEntry, error) {
	return nil, errors.New("not implemented")
}

func (m *fakeMachine) ResolvePath(workspace, path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	return workspace + "/" + path
}

func (m *fakeMachine) ReadFile(path string) ([]byte, error) {
	data, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (m *fakeMachine) ReadFileVersion(path string, _ int64) (machine.FileContent, error) {
	data, err := m.ReadFile(path)
	if err != nil {
		return machine.FileContent{}, err
	}
	sum := sha256.Sum256(data)
	return machine.FileContent{Data: data, Hash: hex.EncodeToString(sum[:])}, nil
}

func (m *fakeMachine) Metadata(string) (machine.FileMetadata, error) {
	return machine.FileMetadata{}, errors.New("not implemented")
}

func (m *fakeMachine) Watch(string) (machine.FileWatch, error) {
	return nil, errors.New("not implemented")
}

func (m *fakeMachine) WriteFile(path string, data []byte) error {
	m.files[path] = append([]byte(nil), data...)
	return nil
}

func (m *fakeMachine) WriteFileIfUnchanged(path string, data []byte, expectedHash string) (string, error) {
	current, exists := m.files[path]
	if expectedHash == machine.AbsentFileHash {
		if exists {
			return "", machine.ErrFileConflict
		}
	} else {
		sum := sha256.Sum256(current)
		if !exists || hex.EncodeToString(sum[:]) != expectedHash {
			return "", machine.ErrFileConflict
		}
	}
	err := m.WriteFile(path, data)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (m *fakeMachine) RemoveFileIfUnchanged(path string, expectedHash string) error {
	current, exists := m.files[path]
	sum := sha256.Sum256(current)
	if !exists || hex.EncodeToString(sum[:]) != expectedHash {
		return machine.ErrFileConflict
	}
	delete(m.files, path)
	return nil
}

func TestApplyPatchToolRegistrationAndCall(t *testing.T) {
	m := &fakeMachine{files: map[string][]byte{
		"/work/update.txt": []byte("before\n"),
		"/work/delete.txt": []byte("remove\n"),
	}}
	service := approvals.New()
	t.Cleanup(func() { _ = service.Close() })
	registry := kerneltools.NewRegistry()
	err := registry.Register(applypatch.New(m, m, service))
	if err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Add File: nested/add.txt
+added
*** Update File: update.txt
@@
-before
+after
*** Delete File: delete.txt
*** End Patch`
	result, err := registry.Call(context.Background(), kerneltools.Call{Policy: permissions.Policy{Unrestricted: true},
		Name:      "apply_patch",
		Arguments: json.RawMessage(`{"patch":` + mustJSON(t, patch) + `}`),
		Workspace: "/work",
		Allow:     []string{"apply_patch"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, "A nested/add.txt") || !strings.Contains(result.Content, "M update.txt") || !strings.Contains(result.Content, "D delete.txt") {
		t.Fatalf("result = %#v", result)
	}
	if string(m.files["/work/nested/add.txt"]) != "added\n" || string(m.files["/work/update.txt"]) != "after\n" {
		t.Fatalf("files = %#v", m.files)
	}
	if _, exists := m.files["/work/delete.txt"]; exists {
		t.Fatal("delete.txt still exists")
	}
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (m *fakeMachine) AgentApplyChanges(_ context.Context, _ permissions.Policy, changes []machine.FileChange) (machine.FileCommit, error) {
	result := machine.FileCommit{Exact: true}
	for _, change := range changes {
		var err error
		if change.Content == nil {
			err = m.RemoveFileIfUnchanged(change.Path, change.ExpectedHash)
		} else {
			_, err = m.WriteFileIfUnchanged(change.Path, []byte(*change.Content), change.ExpectedHash)
		}
		if err != nil {
			return result, err
		}
		result.Completed++
	}
	return result, nil
}
