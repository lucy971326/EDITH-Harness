package conversations_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"harness/internal/appserver"
	"harness/internal/conversations"
	"harness/internal/events"
	"harness/internal/runner"
	"harness/internal/session"
	"harness/internal/session/settings"
)

func TestSessionMethodsUseRealProduct(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
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
	// 并发接口调用仍经同一个 conversations.Service 的空会话复用锁。
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

func TestSessionMenuRenameAndForkLatest(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
	server := newRPCServer(t, fixture)
	defer server.Close()
	workspace := t.TempDir()
	file := filepath.Join(workspace, "unchanged.txt")
	if err := os.WriteFile(file, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := fixture.service.Create(workspace)
	if err != nil {
		t.Fatal(err)
	}
	callRename := func(title string) (appserver.SessionResult, error) {
		raw, callErr := server.Call(t.Context(), "harness/session/rename", mustJSON(t, appserver.RenameSessionParams{SessionID: created.Meta.ID, Title: title}))
		if callErr != nil {
			return appserver.SessionResult{}, callErr
		}
		var result appserver.SessionResult
		callErr = json.Unmarshal(raw, &result)
		return result, callErr
	}
	result, err := callRename("  我的会话  ")
	if err != nil || result.Session.Title != "我的会话" {
		t.Fatalf("rename = %#v, %v", result, err)
	}
	for _, title := range []string{" ", "bad\nname", "bad\n", strings.Repeat("a", 81)} {
		_, err = callRename(title)
		assertMethodError(t, err, appserver.CodeInvalidParams)
	}
	meta, err := fixture.sessions.Meta(created.Meta.ID)
	if err != nil || meta.Title != "我的会话" || !meta.TitleEdited {
		t.Fatalf("durable title = %#v, %v", meta, err)
	}
	empty, err := fixture.service.Create(workspace)
	if err != nil || empty.Meta.ID == created.Meta.ID {
		t.Fatalf("renamed empty session reused: %#v, %v", empty, err)
	}
	_, err = server.Call(t.Context(), "harness/session/fork/latest", mustJSON(t, appserver.SessionIDParams{SessionID: empty.Meta.ID}))
	assertMethodError(t, err, appserver.CodeConflict)

	ended := make(chan runner.RunEvent, 8)
	unsubscribe, err := events.Subscribe(fixture.events, func(_ context.Context, event runner.RunEvent) error {
		if event.Kind == runner.RunEnded {
			ended <- event
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	_, err = fixture.service.Send(t.Context(), conversations.RunInput{SessionID: empty.Meta.ID,
		Message: session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "stop before answer"}}}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	if err = fixture.service.Stop(empty.Meta.ID); err != nil {
		t.Fatal(err)
	}
	waitEnded(t, ended, empty.Meta.ID)
	_, err = server.Call(t.Context(), "harness/session/fork/latest", mustJSON(t, appserver.SessionIDParams{SessionID: empty.Meta.ID}))
	assertMethodError(t, err, appserver.CodeConflict)

	_, err = fixture.service.Send(t.Context(), conversations.RunInput{SessionID: created.Meta.ID,
		Message: session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "first question"}}}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	_, err = server.Call(t.Context(), "harness/session/fork/latest", mustJSON(t, appserver.SessionIDParams{SessionID: created.Meta.ID}))
	assertMethodError(t, err, appserver.CodeConflict)
	result, err = callRename("运行中改名")
	if err != nil || result.Session.Title != "运行中改名" {
		t.Fatalf("rename during run = %#v, %v", result, err)
	}
	fixture.loop.release()
	waitEnded(t, ended, created.Meta.ID)
	meta, err = fixture.sessions.Meta(created.Meta.ID)
	if err != nil || meta.Title != "运行中改名" {
		t.Fatalf("first message replaced title: %#v, %v", meta, err)
	}
	source, err := fixture.service.Snapshot(created.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := server.Call(t.Context(), "harness/session/fork/latest", mustJSON(t, appserver.SessionIDParams{SessionID: created.Meta.ID}))
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(raw, &result)
	if err != nil {
		t.Fatal(err)
	}
	branch, err := fixture.service.Snapshot(result.Session.SessionID)
	if err != nil || len(branch.Entries) != len(source.Entries) || result.Session.Settings.Workspace != workspace {
		t.Fatalf("forked session = %#v, %v", result.Session, err)
	}
	after, err := fixture.service.Snapshot(created.Meta.ID)
	if err != nil || !reflect.DeepEqual(source.Entries, after.Entries) {
		t.Fatalf("source changed after fork: %v", err)
	}
	contents, err := os.ReadFile(file)
	if err != nil || string(contents) != "original" {
		t.Fatalf("workspace file changed: %q, %v", contents, err)
	}
}

func TestSettingsUpdateRejectsInvalidChoicesWithoutStartingOrSaving(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
	created, err := fixture.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	before, err := fixture.settings.For(created.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []appserver.UpdateSettingsParams{
		{SessionID: created.Meta.ID, AgentID: ""},
		{SessionID: created.Meta.ID, AgentID: "default", PermissionMode: "unknown"},
		{SessionID: created.Meta.ID, AgentID: "default", PermissionMode: "approve_for_me"},
		{SessionID: created.Meta.ID, AgentID: "default", Model: "missing", ReasoningEffort: "high"},
		{SessionID: created.Meta.ID, AgentID: "default", Model: "deepseek/deepseek-flash", ReasoningEffort: "missing"},
		{SessionID: created.Meta.ID, AgentID: "missing", Model: "deepseek/deepseek-flash", ReasoningEffort: "high"},
	} {
		_, err = fixture.service.UpdateSettings(t.Context(), input.SessionID, settings.SessionSettings{
			AgentID: input.AgentID, Model: input.Model, ReasoningEffort: input.ReasoningEffort,
			PermissionMode: input.PermissionMode,
		})
		if !errors.Is(err, conversations.ErrInvalidRunSettings) {
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

func TestSettingsUpdatePersistsAndRejectsWhileRunning(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
	server := newRPCServer(t, fixture)
	created, err := fixture.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(`{"sessionID":"` + created.Meta.ID + `","agentID":"default","model":"deepseek/deepseek-flash","reasoningEffort":"high","permissionMode":"read_only"}`)
	raw, err := server.Call(t.Context(), "harness/session/settings/update", params)
	if err != nil {
		t.Fatal(err)
	}
	var result appserver.SessionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Session.Settings.Workspace != created.Settings.Workspace || result.Session.Settings.Model != "deepseek/deepseek-flash" {
		t.Fatalf("updated session = %#v", result.Session)
	}
	// 只切换模型的请求省略模式时，必须保留已经保存的选择。
	params = json.RawMessage(`{"sessionID":"` + created.Meta.ID + `","agentID":"default","model":"deepseek/deepseek-flash","reasoningEffort":"high"}`)
	_, err = server.Call(t.Context(), "harness/session/settings/update", params)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := fixture.settings.For(created.Meta.ID)
	if err != nil || saved.PermissionMode != "read_only" {
		t.Fatalf("permission mode lost: %+v, %v", saved, err)
	}

	handle, err := fixture.runner.Start(t.Context(), created.Meta.ID, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "hold"}}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	_, err = server.Call(t.Context(), "harness/session/settings/update", params)
	assertMethodError(t, err, appserver.CodeConflict)
	fixture.loop.release()
	handle.Wait()
}

func TestSendAcceptsValidatedImageAndRejectsBadImage(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
	server := newRPCServer(t, fixture)
	created, err := fixture.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.UpdateSettings(t.Context(), created.Meta.ID, settings.SessionSettings{
		AgentID: "default", Model: "xiaomi/mimo-v2.6-pro", ReasoningEffort: "on",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 1x1 PNG；图片与文字使用同一条 UserMessage 落账。
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	raw, err := server.Call(t.Context(), "harness/session/send", json.RawMessage(`{"sessionID":"`+created.Meta.ID+`","images":[{"mime":"image/png","data":"`+png+`"}]}`))
	if err != nil || string(raw) != `{"mode":"started"}` {
		t.Fatalf("image send = %s %v", raw, err)
	}
	fixture.loop.waitStarted(t)
	fixture.loop.release()
	waitIdle(t, fixture.runner, created.Meta.ID)
	snapshot, err := fixture.service.Snapshot(created.Meta.ID)
	if err != nil || snapshot.Entries[0].Message.Blocks[0].Media == nil {
		t.Fatalf("image history = %#v %v", snapshot.Entries, err)
	}

	for _, params := range []string{
		`{"sessionID":"` + created.Meta.ID + `","images":[{"mime":"image/png","data":"%%%"}]}`,
		`{"sessionID":"` + created.Meta.ID + `","images":[{"mime":"image/jpeg","data":"` + png + `"}]}`,
	} {
		_, err = server.Call(t.Context(), "harness/session/send", json.RawMessage(params))
		assertMethodError(t, err, appserver.CodeInvalidParams)
	}
	tooLarge := make([]byte, 2<<20+1)
	tooLargeParams, err := json.Marshal(appserver.SendParams{
		SessionID: created.Meta.ID,
		Images:    []appserver.ImageInput{{MIME: "image/png", Data: base64.StdEncoding.EncodeToString(tooLarge)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Call(t.Context(), "harness/session/send", tooLargeParams)
	assertMethodError(t, err, appserver.CodeInvalidParams)

	five := make([]appserver.ImageInput, 5)
	for index := range five {
		five[index] = appserver.ImageInput{MIME: "image/png", Data: png}
	}
	fiveParams, err := json.Marshal(appserver.SendParams{SessionID: created.Meta.ID, Images: five})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Call(t.Context(), "harness/session/send", fiveParams)
	assertMethodError(t, err, appserver.CodeInvalidParams)

	_, err = fixture.service.UpdateSettings(t.Context(), created.Meta.ID, settings.SessionSettings{
		AgentID: "default", Model: "deepseek/deepseek-v4-pro", ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Call(t.Context(), "harness/session/send", json.RawMessage(`{"sessionID":"`+created.Meta.ID+`","images":[{"mime":"image/png","data":"`+png+`"}]}`))
	assertMethodError(t, err, appserver.CodeInvalidParams)
}

func waitIdle(t *testing.T, runner *runner.Runner, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, running := runner.State(sessionID); !running {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("run did not become idle")
}

func TestSendExpectedRunDoesNotStartOrSteerAnotherRun(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
	server := newRPCServer(t, fixture)
	created, err := fixture.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := created.Meta.ID
	params := json.RawMessage(`{"sessionID":"` + id + `","text":"late","expectedRunID":"old"}`)
	_, err = server.Call(t.Context(), "harness/session/send", params)
	assertMethodError(t, err, appserver.CodeConflict)
	before, err := fixture.service.Snapshot(id)
	if err != nil || len(before.Entries) != 0 || len(before.Runs) != 0 {
		t.Fatalf("idle guard started a run: %+v %v", before, err)
	}

	setup, err := fixture.settings.For(id)
	if err != nil {
		t.Fatal(err)
	}
	setup.Model, setup.ReasoningEffort = "deepseek/deepseek-flash", "high"
	err = fixture.settings.Put(id, setup)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := fixture.runner.Start(t.Context(), id, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "first"}}})
	if err != nil {
		t.Fatal(err)
	}
	invocation := fixture.loop.waitStarted(t)
	_, err = server.Call(t.Context(), "harness/session/send", params)
	assertMethodError(t, err, appserver.CodeConflict)
	params = json.RawMessage(`{"sessionID":"` + id + `","text":"steer","expectedRunID":"` + handle.RunID() + `"}`)
	type callOutcome struct {
		raw json.RawMessage
		err error
	}
	called := make(chan callOutcome, 1)
	go func() {
		raw, callErr := server.Call(t.Context(), "harness/session/send", params)
		called <- callOutcome{raw: raw, err: callErr}
	}()
	select {
	case <-invocation.InputSignal():
	case <-time.After(time.Second):
		t.Fatal("Steer did not reach Runner")
	}
	fixture.loop.release()
	outcome := <-called
	raw, err := outcome.raw, outcome.err
	if err != nil || string(raw) != `{"mode":"steered"}` {
		t.Fatalf("matching steer: %s %v", raw, err)
	}
	handle.Wait()
	before, err = fixture.service.Snapshot(id)
	if err != nil || len(before.Entries) != 3 {
		t.Fatalf("unexpected ledger: %+v %v", before, err)
	}
	_, err = server.Call(t.Context(), "harness/session/send", params)
	assertMethodError(t, err, appserver.CodeConflict)
	after, err := fixture.service.Snapshot(id)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("ended guard changed history: %+v %v", after, err)
	}
	_, err = server.Call(t.Context(), "harness/session/send", json.RawMessage(`{"sessionID":"`+id+`","text":"x","expectedRunID":""}`))
	assertMethodError(t, err, appserver.CodeInvalidParams)
}

func TestServerCloseRejectsCalls(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
	server := newRPCServer(t, fixture)
	defer server.Close()
	err := server.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Call(context.Background(), "harness/session/list", json.RawMessage(`{}`))
	assertMethodError(t, err, appserver.CodeConflict)
}

func TestModelListUsesPublicServiceWithoutSecrets(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
	server := newRPCServer(t, fixture)
	err := server.BindModels(fixture.models)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := server.Call(t.Context(), "model/list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(appserver.ModelListResult{Models: fixture.models.Models()})
	if err != nil || string(raw) != string(want) {
		t.Fatalf("model list: %s %v", raw, err)
	}
	var data struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	err = json.Unmarshal(raw, &data)
	if err != nil || len(data.Models) == 0 {
		t.Fatalf("empty fixture models: %s %v", raw, err)
	}
	for _, model := range data.Models {
		if len(model) != 5 || model["id"] == nil || model["provider"] == nil || model["contextWindow"] == nil || model["vision"] == nil || model["reasoningEfforts"] == nil {
			t.Fatalf("unexpected public model fields: %s", raw)
		}
	}
	_, err = server.Call(t.Context(), "model/list", json.RawMessage(`{"apiKey":"x"}`))
	assertMethodError(t, err, appserver.CodeInvalidParams)
}

func TestAgentMethodsUsePublicServiceAndProtectDeletes(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.close()
	server := newRPCServer(t, fixture)
	err := server.BindAgents(fixture.agents)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := server.Call(t.Context(), "agent/list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var listed appserver.AgentListResult
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Agents) != 1 || listed.Agents[0].ID != "default" {
		t.Fatalf("agent list = %#v", listed)
	}

	raw, err = server.Call(t.Context(), "agent/save", json.RawMessage(`{"name":"Reviewer","systemPrompt":"review","tools":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	var saved appserver.AgentSaveResult
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Agent.ID == "" || saved.Agent.Name != "Reviewer" {
		t.Fatalf("saved = %#v", saved)
	}
	raw, err = server.Call(t.Context(), "agent/save", json.RawMessage(`{"id":"`+saved.Agent.ID+`","name":"Updated","systemPrompt":"review","tools":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Agent.Name != "Updated" {
		t.Fatalf("updated = %s %v", raw, err)
	}
	_, err = server.Call(t.Context(), "agent/save", json.RawMessage(`{"id":"missing","name":"Missing","systemPrompt":"","tools":[]}`))
	assertMethodError(t, err, appserver.CodeNotFound)

	created, err := fixture.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.UpdateSettings(t.Context(), created.Meta.ID, settings.SessionSettings{AgentID: saved.Agent.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Call(t.Context(), "agent/delete", json.RawMessage(`{"agentID":"`+saved.Agent.ID+`"}`))
	assertMethodError(t, err, appserver.CodeConflict)
	_, err = server.Call(t.Context(), "agent/delete", json.RawMessage(`{"agentID":"default"}`))
	assertMethodError(t, err, appserver.CodeConflict)
	_, err = server.Call(t.Context(), "agent/save", json.RawMessage(`{"name":"Bad","kind":"missing","systemPrompt":"","tools":[]}`))
	assertMethodError(t, err, appserver.CodeInvalidParams)
	_, err = server.Call(t.Context(), "agent/save", json.RawMessage(`{"name":"Bad tool","systemPrompt":"","tools":["missing"]}`))
	assertMethodError(t, err, appserver.CodeInvalidParams)
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
	err := server.BindHarness(fixture.service, fixture.runner, fixture.events)
	if err != nil {
		t.Fatal(err)
	}
	return server
}
