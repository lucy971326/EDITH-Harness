package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"harness/appserver"
	"harness/kernel/host"
)

func TestSessionMethodsUseRealProduct(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
	server, err := host.Resolve[*appserver.RPCServer](fixture.host, "appServer")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	raw, err := server.Call(context.Background(), ListMethod().Name, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"sessions":[]}` {
		t.Fatalf("empty list = %s", raw)
	}

	workspace := t.TempDir()
	params, err := json.Marshal(CreateParams{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	// 并发接口调用仍经同一个 Product 的空会话复用锁。
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := server.Call(context.Background(), CreateMethod().Name, params)
			if err != nil {
				t.Error(err)
				return
			}
			var result SessionResult
			err = json.Unmarshal(raw, &result)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- result.Session.SessionID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for actual := range ids {
		if id != "" && actual != id {
			t.Fatal("concurrent create did not reuse empty session")
		}
		id = actual
	}
	if id == "" {
		t.Fatal("no session returned")
	}
	params, err = json.Marshal(SessionIDParams{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = server.Call(context.Background(), GetMethod().Name, params)
	if err != nil {
		t.Fatal(err)
	}
	var result SessionResult
	err = json.Unmarshal(raw, &result)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := fixture.service.Session(id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Session, sessionView(actual)) {
		t.Fatalf("wire session = %+v, product = %+v", result, actual)
	}
	var envelope struct {
		Session struct {
			CreatedAt string `json:"createdAt"`
		} `json:"session"`
	}
	err = json.Unmarshal(raw, &envelope)
	if err != nil {
		t.Fatal(err)
	}
	_, err = time.Parse(time.RFC3339, envelope.Session.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, params string
		code         appserver.ErrorCode
	}{
		{GetMethod().Name, `{"sessionID":"missing"}`, appserver.CodeNotFound},
		{GetMethod().Name, `{"sessionID":""}`, appserver.CodeInvalidParams},
		{CreateMethod().Name, `{"workspace":"relative"}`, appserver.CodeInvalidParams},
		{ListMethod().Name, `{"filter":"all"}`, appserver.CodeInvalidParams},
		{"harness/session/start", `{}`, appserver.CodeUnknownMethod},
	} {
		_, err = server.Call(context.Background(), test.name, json.RawMessage(test.params))
		assertMethodError(t, err, test.code)
	}
}

func TestMethodErrorsDoNotConfuseMissingFilesWithMissingSession(t *testing.T) {
	if mapped := methodError(os.ErrNotExist); mapped != os.ErrNotExist {
		t.Fatal("storage failure was reclassified as missing session")
	}
	assertMethodError(t, methodError(ErrSessionNotFound), appserver.CodeNotFound)
	assertMethodError(t, methodError(ErrWorkspace), appserver.CodeInvalidParams)
	assertMethodError(t, methodError(ErrInvalidRunSettings), appserver.CodeInvalidParams)
}

func TestSendRejectsInvalidSettingsWithoutStartingOrSaving(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
	created, err := fixture.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	before, err := fixture.settings.For(created.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	handlers := &runHandlers{product: fixture.service, events: fixture.events}
	for _, input := range []SendParams{
		{SessionID: created.Meta.ID, Text: "hello"},
		{SessionID: created.Meta.ID, Text: "hello", Model: "missing", ReasoningEffort: "high"},
		{SessionID: created.Meta.ID, Text: "hello", Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "missing"},
		{SessionID: created.Meta.ID, Text: "hello", Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high", AgentID: "missing"},
	} {
		_, err = handlers.send(t.Context(), input)
		assertMethodError(t, err, appserver.CodeInvalidParams)
		if !errors.Is(err, ErrInvalidRunSettings) {
			t.Fatalf("settings error classification lost: %v", err)
		}
	}
	after, err := fixture.settings.For(created.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("invalid settings were saved")
	}
	snapshot, err := fixture.service.Snapshot(created.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) != 0 || len(snapshot.Runs) != 0 {
		t.Fatal("invalid input started a run")
	}
}

func TestProductInstallFailureCleanupClosesCalls(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
	server, err := host.Resolve[*appserver.RPCServer](fixture.host, "appServer")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	// 真实产品重复安装失败后，由入口关闭 RPCServer。
	err = fixture.host.Install(NewPlugin())
	if err == nil {
		t.Fatal("duplicate product installed")
	}
	err = server.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Call(context.Background(), ListMethod().Name, json.RawMessage(`{}`))
	assertMethodError(t, err, appserver.CodeConflict)
}

func assertMethodError(t *testing.T, err error, code appserver.ErrorCode) {
	t.Helper()
	var public *appserver.Error
	if !errors.As(err, &public) || public.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}
