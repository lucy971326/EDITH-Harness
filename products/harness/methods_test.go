package harness_test

import (
	"context"
	"encoding/json"
	"errors"
	"harness/products/harness"
	"reflect"
	"sync"
	"testing"
	"time"

	"harness/appserver"
	"harness/kernel/session"
)

func TestSessionMethodsUseRealProduct(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
	server := newRPCServer(t, fixture)
	defer server.Close()
	raw, err := server.Call(context.Background(), "harness/session/list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"sessions":[]}` {
		t.Fatalf("empty list = %s", raw)
	}

	workspace := t.TempDir()
	params, err := json.Marshal(appserver.CreateParams{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	// 并发接口调用仍经同一个 harness.Product 的空会话复用锁。
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := server.Call(context.Background(), "harness/session/create", params)
			if err != nil {
				t.Error(err)
				return
			}
			var result appserver.SessionResult
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
	params, err = json.Marshal(appserver.SessionIDParams{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = server.Call(context.Background(), "harness/session/get", params)
	if err != nil {
		t.Fatal(err)
	}
	var result appserver.SessionResult
	err = json.Unmarshal(raw, &result)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := fixture.service.Session(id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Session, appserver.SessionView{SessionID: actual.Meta.ID, Title: actual.Meta.Title, CreatedAt: actual.Meta.CreatedAt, Settings: actual.Settings}) {
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
		{"harness/session/get", `{"sessionID":"missing"}`, appserver.CodeNotFound},
		{"harness/session/get", `{"sessionID":""}`, appserver.CodeInvalidParams},
		{"harness/session/create", `{"workspace":"relative"}`, appserver.CodeInvalidParams},
		{"harness/session/list", `{"filter":"all"}`, appserver.CodeInvalidParams},
		{"workspace/select", `{"extra":true}`, appserver.CodeInvalidParams},
		{"harness/session/start", `{}`, appserver.CodeUnknownMethod},
		{"harness/session/send", `{"sessionID":"missing","text":"  "}`, appserver.CodeNotFound},
		{"harness/session/send", `{"sessionID":"` + id + `","text":"  "}`, appserver.CodeInvalidParams},
	} {
		_, err = server.Call(context.Background(), test.name, json.RawMessage(test.params))
		assertMethodError(t, err, test.code)
	}
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
	for _, input := range []appserver.SendParams{
		{SessionID: created.Meta.ID, Text: "hello"},
		{SessionID: created.Meta.ID, Text: "hello", Model: "missing", ReasoningEffort: "high"},
		{SessionID: created.Meta.ID, Text: "hello", Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "missing"},
		{SessionID: created.Meta.ID, Text: "hello", Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high", AgentID: "missing"},
	} {
		_, err = fixture.service.Send(t.Context(), harness.RunInput{
			SessionID: input.SessionID, AgentID: input.AgentID, Model: input.Model, ReasoningEffort: input.ReasoningEffort,
			Message: session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: input.Text}}},
		})
		if !errors.Is(err, harness.ErrInvalidRunSettings) {
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
	server := newRPCServer(t, fixture)
	defer server.Close()
	// 真实产品重复安装失败后，由入口关闭 app-server。
	err := fixture.host.Install(harness.NewPlugin())
	if err == nil {
		t.Fatal("duplicate product installed")
	}
	err = server.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Call(context.Background(), "harness/session/list", json.RawMessage(`{}`))
	assertMethodError(t, err, appserver.CodeConflict)
}

func assertMethodError(t *testing.T, err error, code appserver.ErrorCode) {
	t.Helper()
	var public *appserver.Error
	if !errors.As(err, &public) || public.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}

func newRPCServer(t *testing.T, fixture testFixture) *appserver.Server {
	t.Helper()
	server := appserver.New()
	t.Cleanup(func() { _ = server.Close() })
	err := server.BindHarness(fixture.service, fixture.events)
	if err != nil {
		t.Fatal(err)
	}
	return server
}
