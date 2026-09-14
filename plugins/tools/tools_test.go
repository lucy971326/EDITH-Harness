package tools_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"harness/kernel/host"
	"harness/kernel/machine"
	kerneltools "harness/kernel/tools"
	"harness/plugins/tools/bash"
	"harness/plugins/tools/edit"
	"harness/plugins/tools/read"
	"harness/plugins/tools/write"
)

// 活对象。工具插件测试使用的内存机器。
type fakeMachine struct {
	files    map[string][]byte
	onRead   func()
	runDir   string
	runArgv  []string
	stdout   []byte
	stderr   []byte
	runError error
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
		return nil, errors.New("file not found")
	}
	if m.onRead != nil {
		m.onRead()
	}
	return data, nil
}

func (m *fakeMachine) ReadFileVersion(path string, _ int64) (machine.FileContent, error) {
	data, err := m.ReadFile(path)
	if err != nil {
		return machine.FileContent{}, err
	}
	sum := sha256.Sum256(data)
	return machine.FileContent{Data: append([]byte(nil), data...), Hash: hex.EncodeToString(sum[:])}, nil
}

func (m *fakeMachine) Metadata(string) (machine.FileMetadata, error) {
	return machine.FileMetadata{}, errors.New("not implemented")
}

func (m *fakeMachine) WriteFile(path string, data []byte) error {
	m.files[path] = append([]byte(nil), data...)
	return nil
}

func (m *fakeMachine) WriteFileIfUnchanged(path string, data []byte, expectedHash string) (string, error) {
	current, err := m.ReadFileVersion(path, 0)
	if err != nil {
		return "", err
	}
	if current.Hash != expectedHash {
		return "", machine.ErrFileConflict
	}
	if err := m.WriteFile(path, data); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (m *fakeMachine) Watch(string) (machine.FileWatch, error) {
	return nil, errors.New("not implemented")
}

func (m *fakeMachine) Run(_ context.Context, dir string, argv []string) ([]byte, []byte, error) {
	m.runDir = dir
	m.runArgv = append([]string(nil), argv...)
	return m.stdout, m.stderr, m.runError
}

func TestCoreTools(t *testing.T) {
	m := &fakeMachine{files: map[string][]byte{
		"/work/read.txt": []byte("hello"),
		"/work/edit.txt": []byte("before\nafter"),
		"/work/many.txt": []byte("same same"),
	}}
	registry := installTools(t, m)
	allow := []string{"read", "write", "edit", "bash"}

	result := call(t, registry, allow, "read", `{"path":"read.txt"}`)
	if result.Content != "hello" || result.IsError {
		t.Fatalf("read result = %#v", result)
	}

	result = call(t, registry, allow, "write", `{"path":"nested/write.txt","content":"written"}`)
	if result.IsError || string(m.files["/work/nested/write.txt"]) != "written" {
		t.Fatalf("write result = %#v, files = %#v", result, m.files)
	}

	result = call(t, registry, allow, "edit", `{"path":"edit.txt","oldText":"before","newText":"after"}`)
	if result.IsError || string(m.files["/work/edit.txt"]) != "after\nafter" {
		t.Fatalf("edit result = %#v, content = %q", result, m.files["/work/edit.txt"])
	}

	result = call(t, registry, allow, "edit", `{"path":"many.txt","oldText":"same","newText":"new"}`)
	if !result.IsError || string(m.files["/work/many.txt"]) != "same same" {
		t.Fatalf("multiple edit result = %#v, content = %q", result, m.files["/work/many.txt"])
	}

	m.stdout = []byte("out")
	m.stderr = []byte("err")
	m.runError = errors.New("exit status 1")
	result = call(t, registry, allow, "bash", `{"command":"false"}`)
	if !result.IsError || !strings.Contains(result.Content, "stdout:\nout") || !strings.Contains(result.Content, "stderr:\nerr") {
		t.Fatalf("bash result = %#v", result)
	}
	if m.runDir != "/work" || strings.Join(m.runArgv, " ") != "bash -c false" {
		t.Fatalf("bash run = dir %q argv %#v", m.runDir, m.runArgv)
	}
}

func TestEdit_doesNotWriteAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &fakeMachine{files: map[string][]byte{
		"/work/edit.txt": []byte("before"),
	}}
	m.onRead = cancel
	registry := installTools(t, m)

	_, err := registry.Call(ctx, kerneltools.Call{
		Name:      "edit",
		Arguments: json.RawMessage(`{"path":"edit.txt","oldText":"before","newText":"after"}`),
		Workspace: "/work",
		Allow:     []string{"edit"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if string(m.files["/work/edit.txt"]) != "before" {
		t.Fatalf("file changed after cancellation: %q", m.files["/work/edit.txt"])
	}
}

func TestEdit_doesNotOverwriteConcurrentChange(t *testing.T) {
	m := &fakeMachine{files: map[string][]byte{
		"/work/edit.txt": []byte("before"),
	}}
	m.onRead = func() {
		m.files["/work/edit.txt"] = []byte("changed elsewhere")
		m.onRead = nil
	}
	registry := installTools(t, m)

	result := call(t, registry, []string{"edit"}, "edit", `{"path":"edit.txt","oldText":"before","newText":"after"}`)
	if !result.IsError {
		t.Fatalf("edit result = %#v, want conflict", result)
	}
	if string(m.files["/work/edit.txt"]) != "changed elsewhere" {
		t.Fatalf("concurrent content was overwritten: %q", m.files["/work/edit.txt"])
	}
}

func installTools(t *testing.T, m machine.FileSystem) kerneltools.Tools {
	t.Helper()
	h := host.NewHost()
	err := h.RegisterService("machine", m)
	if err != nil {
		t.Fatal(err)
	}
	for _, plugin := range []host.Plugin{kerneltools.NewPlugin(), read.New(), write.New(), edit.New(), bash.New()} {
		err = h.Install(plugin)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		err := h.Close()
		if err != nil {
			t.Fatal(err)
		}
	})
	registry, err := host.Resolve[kerneltools.Tools](h, "tools")
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func call(t *testing.T, registry kerneltools.Tools, allow []string, name string, arguments string) kerneltools.Result {
	t.Helper()
	result, err := registry.Call(context.Background(), kerneltools.Call{
		Name:      name,
		Arguments: json.RawMessage(arguments),
		Workspace: "/work",
		Allow:     allow,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
