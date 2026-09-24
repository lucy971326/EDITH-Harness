package appserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	machinelocal "harness/plugins/machine/local"
)

func TestFilesystemReadWriteConflictAndLimit(t *testing.T) {
	server := newFilesystemServer(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "中文.bin")
	want := []byte{0, 1, 2, 0xff}
	err := os.WriteFile(path, want, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	read := callFilesystem[ReadFileResult](t, server, readFileMethod, FilePathParams{Path: path})
	got, err := base64.StdEncoding.DecodeString(read.DataBase64)
	if err != nil || !bytes.Equal(got, want) || len(read.Hash) != 64 {
		t.Fatalf("read = %#v, data=%v, error=%v", read, got, err)
	}
	updated := []byte("updated")
	written := callFilesystem[WriteFileResult](t, server, writeFileMethod, WriteFileParams{
		Path:         path,
		DataBase64:   base64.StdEncoding.EncodeToString(updated),
		ExpectedHash: read.Hash,
	})
	if len(written.Hash) != 64 {
		t.Fatalf("write = %#v", written)
	}
	disk, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(disk, updated) {
		t.Fatalf("disk = %q, error=%v", disk, err)
	}

	err = os.WriteFile(path, []byte("external"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = server.Call(context.Background(), writeFileMethod, mustJSON(t, WriteFileParams{
		Path:         path,
		DataBase64:   base64.StdEncoding.EncodeToString([]byte("stale")),
		ExpectedHash: written.Hash,
	}))
	assertCode(t, err, CodeConflict)

	large := bytes.Repeat([]byte("x"), maxEditorFileBytes+1)
	err = os.WriteFile(path, large, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Call(context.Background(), readFileMethod, mustJSON(t, FilePathParams{Path: path}))
	assertCode(t, err, CodeInvalidParams)
}

func TestFilesystemWatchUsesSubscriptionEnvelopeAndUnsubscribe(t *testing.T) {
	server := newFilesystemServer(t)
	_, url := startTestSocket(t, server)
	ws := dialTestSocket(t, url)
	initializeSocket(t, ws)

	path := filepath.Join(t.TempDir(), "note.txt")
	err := os.WriteFile(path, []byte("before"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	rawParams := mustJSON(t, FilePathParams{Path: path})
	request := `{"jsonrpc":"2.0","id":2,"method":"fs/watch","params":` + string(rawParams) + `}`
	response := socketRequest(t, ws, request)
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var watched WatchResult
	err = json.Unmarshal(response.Result, &watched)
	if err != nil || watched.SubscriptionID == "" {
		t.Fatalf("watch result = %s, error=%v", response.Result, err)
	}

	err = os.WriteFile(path, []byte("changed"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, raw, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var notification struct {
		Method string `json:"method"`
		Params struct {
			SubscriptionID string           `json:"subscriptionID"`
			Event          FileChangedEvent `json:"event"`
		} `json:"params"`
	}
	err = json.Unmarshal(raw, &notification)
	if err != nil {
		t.Fatal(err)
	}
	if notification.Method != changedMethod || notification.Params.SubscriptionID != watched.SubscriptionID || len(notification.Params.Event.ChangedPaths) == 0 {
		t.Fatalf("notification = %s", raw)
	}

	unsubscribe := `{"jsonrpc":"2.0","id":3,"method":"server/unsubscribe","params":{"subscriptionID":"` + watched.SubscriptionID + `"}}`
	response = socketRequest(t, ws, unsubscribe)
	if response.Error != nil || string(response.Result) != `{}` {
		t.Fatalf("unsubscribe = %#v", response)
	}
}

func newFilesystemServer(t *testing.T) *Server {
	t.Helper()
	filesystem, err := machinelocal.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = filesystem.Close() })

	server := New()
	err = server.BindFilesystem(filesystem)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Error(err)
		}
	})
	return server
}

func callFilesystem[Output any](t *testing.T, server *Server, method string, params any) Output {
	t.Helper()
	raw, err := server.Call(context.Background(), method, mustJSON(t, params))
	if err != nil {
		t.Fatal(err)
	}
	var result Output
	err = json.Unmarshal(raw, &result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
