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

func TestSessionMethodsUseRealProductAndMatchGeneratedCatalog(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
	server, err := host.Resolve[*appserver.RPCServer](fixture.host, "appServer")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	definitions, err := Definitions()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(server.Catalog(), definitions) {
		t.Fatal("runtime and generator catalogs diverged")
	}
	err = server.Freeze()
	if err != nil {
		t.Fatal(err)
	}
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
	params, err = json.Marshal(GetParams{SessionID: id})
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
}

func TestProductInstallFailureLeavesEntryUnopened(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.host.Close()
	server, err := host.Resolve[*appserver.RPCServer](fixture.host, "appServer")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	// 真实产品重复安装在组装期失败；它不能把入口偷偷冻结并开放。
	err = fixture.host.Install(NewPlugin())
	if err == nil {
		t.Fatal("duplicate product installed")
	}
	_, err = server.Call(context.Background(), ListMethod().Name, json.RawMessage(`{}`))
	assertMethodError(t, err, appserver.CodeConflict)
	err = server.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = server.Freeze()
	if err == nil {
		t.Fatal("failed assembly reopened")
	}
}

func assertMethodError(t *testing.T, err error, code appserver.ErrorCode) {
	t.Helper()
	var public *appserver.Error
	if !errors.As(err, &public) || public.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
}
