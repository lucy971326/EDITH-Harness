package harness_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"harness/appserver"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/products/harness"
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

func TestSettingsUpdateRejectsInvalidChoicesWithoutStartingOrSaving(t *testing.T) {
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
	for _, input := range []appserver.UpdateSettingsParams{
		{SessionID: created.Meta.ID, AgentID: ""},
		{SessionID: created.Meta.ID, AgentID: "default", Model: "missing", ReasoningEffort: "high"},
		{SessionID: created.Meta.ID, AgentID: "default", Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "missing"},
		{SessionID: created.Meta.ID, AgentID: "missing", Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high"},
	} {
		_, err = fixture.service.UpdateSettings(t.Context(), input.SessionID, settings.SessionSettings{
			AgentID: input.AgentID, Model: input.Model, ReasoningEffort: input.ReasoningEffort,
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

func TestSettingsUpdatePersistsAndRejectsWhileRunning(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
	server := newRPCServer(t, fixture)
	created, err := fixture.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(`{"sessionID":"` + created.Meta.ID + `","agentID":"default","model":"deepseek/deepseek-v4-flash","reasoningEffort":"high"}`)
	raw, err := server.Call(t.Context(), "harness/session/settings/update", params)
	if err != nil {
		t.Fatal(err)
	}
	var result appserver.SessionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Session.Settings.Workspace != created.Settings.Workspace || result.Session.Settings.Model != "deepseek/deepseek-v4-flash" {
		t.Fatalf("updated session = %#v", result.Session)
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
	defer fixture.host.Close()
	server := newRPCServer(t, fixture)
	created, err := fixture.service.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.UpdateSettings(t.Context(), created.Meta.ID, settings.SessionSettings{
		AgentID: "default", Model: "google/gemini-3.5-flash-lite", ReasoningEffort: "low",
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
		AgentID: "default", Model: "deepseek/deepseek-v4-flash", ReasoningEffort: "high",
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
	defer fixture.host.Close()
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
	setup.Model, setup.ReasoningEffort = "deepseek/deepseek-v4-flash", "high"
	err = fixture.settings.Put(id, setup)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := fixture.runner.Start(t.Context(), id, session.UserMessage{Blocks: []session.Block{{Kind: "text", Text: "first"}}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.loop.waitStarted(t)
	_, err = server.Call(t.Context(), "harness/session/send", params)
	assertMethodError(t, err, appserver.CodeConflict)
	params = json.RawMessage(`{"sessionID":"` + id + `","text":"steer","expectedRunID":"` + handle.RunID() + `"}`)
	raw, err := server.Call(t.Context(), "harness/session/send", params)
	if err != nil || string(raw) != `{"mode":"steered"}` {
		t.Fatalf("matching steer: %s %v", raw, err)
	}
	fixture.loop.release()
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

func TestModelListUsesPublicServiceWithoutSecrets(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
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
		if len(model) != 4 || model["id"] == nil || model["contextWindow"] == nil || model["vision"] == nil || model["reasoningEfforts"] == nil {
			t.Fatalf("unexpected public model fields: %s", raw)
		}
	}
	_, err = server.Call(t.Context(), "model/list", json.RawMessage(`{"apiKey":"x"}`))
	assertMethodError(t, err, appserver.CodeInvalidParams)
}

func TestAgentMethodsUsePublicServiceAndProtectDeletes(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
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
	if len(listed.Agents) != 1 || listed.Agents[0].ID != "default" || len(listed.Kinds) == 0 {
		t.Fatalf("agent list = %#v", listed)
	}

	raw, err = server.Call(t.Context(), "agent/save", json.RawMessage(`{"name":"Reviewer","kind":"react","systemPrompt":"review","tools":[]}`))
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
	raw, err = server.Call(t.Context(), "agent/save", json.RawMessage(`{"id":"`+saved.Agent.ID+`","name":"Updated","kind":"react","systemPrompt":"review","tools":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Agent.Name != "Updated" {
		t.Fatalf("updated = %s %v", raw, err)
	}
	_, err = server.Call(t.Context(), "agent/save", json.RawMessage(`{"id":"missing","name":"Missing","kind":"react","systemPrompt":"","tools":[]}`))
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
	_, err = server.Call(t.Context(), "agent/save", json.RawMessage(`{"name":"Bad tool","kind":"react","systemPrompt":"","tools":["missing"]}`))
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
	err := server.BindHarness(fixture.service, fixture.events)
	if err != nil {
		t.Fatal(err)
	}
	return server
}
