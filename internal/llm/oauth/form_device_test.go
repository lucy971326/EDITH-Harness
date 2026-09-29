package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"harness/internal/persist"
)

// 设备码先返回待确认，再轮询成功；刷新后必须持久保存轮换的 refresh token。
func TestDeviceAuthPersistsRefreshAndLogout(t *testing.T) {
	polls := 0
	refreshes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device":
			_, _ = fmt.Fprint(w, `{"device_code":"device","user_code":"ABCD","verification_uri":"https://www.kimi.com/code","interval":1,"expires_in":30}`)
		case "/token":
			switch r.Form.Get("grant_type") {
			case "urn:ietf:params:oauth:grant-type:device_code":
				polls++
				if polls == 1 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprint(w, `{"error":"authorization_pending"}`)
					return
				}
				_, _ = fmt.Fprint(w, `{"access_token":"access-1","refresh_token":"refresh-1","expires_in":30}`)
			case "refresh_token":
				refreshes++
				if r.Form.Get("refresh_token") != "refresh-1" {
					t.Error("wrong refresh token")
				}
				_, _ = fmt.Fprint(w, `{"access_token":"access-2","refresh_token":"refresh-2","expires_in":3600}`)
			}
		}
	}))
	defer server.Close()
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	driver := &FormDevice{Name: "Kimi", ClientID: "client", DeviceURL: server.URL + "/device", TokenURL: server.URL + "/token", VerificationDomain: "kimi.com"}
	auth, err := NewDeviceAuth(files, "kimi-test", driver)
	if err != nil {
		t.Fatal(err)
	}
	view, err := auth.Start()
	if err != nil || view.UserCode != "ABCD" {
		t.Fatalf("start: %+v %v", view, err)
	}
	deadline := time.After(5 * time.Second)
	for !auth.Authenticated() {
		select {
		case <-deadline:
			t.Fatal("device login timed out")
		case <-time.After(25 * time.Millisecond):
		}
	}
	token, err := auth.Token(t.Context())
	if err != nil || token != "access-2" || refreshes != 1 {
		t.Fatalf("refresh: %q, %d, %v", token, refreshes, err)
	}
	auth.Close()
	reloaded, err := NewDeviceAuth(files, "kimi-test", driver)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Authenticated() {
		t.Fatal("refreshed credential not persisted")
	}
	if _, err := reloaded.Logout(); err != nil {
		t.Fatal(err)
	}
	reloaded.Close()
	after, err := NewDeviceAuth(files, "kimi-test", driver)
	if err != nil || after.Authenticated() {
		t.Fatalf("logout not persisted: %v", err)
	}
	after.Close()
}

func TestFormDeviceRejectsUnsafeURLAndKeepsRefreshOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/device" {
			_, _ = fmt.Fprint(w, `{"device_code":"device","user_code":"ABCD","verification_uri":"file:///tmp/secret","expires_in":30}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	defer server.Close()
	driver := &FormDevice{Name: "xAI", ClientID: "client", DeviceURL: server.URL + "/device", TokenURL: server.URL + "/token", ReuseRefresh: true, VerificationDomain: "x.ai"}
	if _, err := driver.Begin(context.Background()); err == nil || !strings.Contains(err.Error(), "不可信") {
		t.Fatalf("unsafe URL accepted: %v", err)
	}
	previous := Credential{Access: "old-access", Refresh: "old-refresh", Expires: time.Now()}
	if _, err := driver.Refresh(context.Background(), previous); !errors.Is(err, ErrLoginExpired) {
		t.Fatalf("expected expired login: %v", err)
	}
	if previous.Refresh != "old-refresh" {
		t.Fatal("previous credential changed")
	}
}

func TestExpiredRefreshClearsStoredLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	defer server.Close()
	files, err := persist.NewFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(files, "expired")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write([]byte(`{"access":"old","refresh":"expired","expires":"2020-01-01T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	driver := &FormDevice{Name: "xAI", ClientID: "client", TokenURL: server.URL, ReuseRefresh: true, VerificationDomain: "x.ai"}
	auth, err := NewDeviceAuth(files, "expired", driver)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Token(t.Context()); !errors.Is(err, ErrLoginExpired) {
		t.Fatalf("expected expired token: %v", err)
	}
	if auth.Authenticated() {
		t.Fatal("expired login still appears connected")
	}
	auth.Close()
	reloaded, err := NewDeviceAuth(files, "expired", driver)
	if err != nil || reloaded.Authenticated() {
		t.Fatalf("expired token persisted: %v", err)
	}
	reloaded.Close()
}
