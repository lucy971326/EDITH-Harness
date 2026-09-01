package chat

import (
	"context"
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"harness/kernel/host"
	"harness/kernel/persist"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/surface/web"
)

func TestPluginRendersFullPageAndHTMXMain(t *testing.T) {
	h := host.NewHost()
	err := h.Install(&persist.Plugin{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&session.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	webPlugin := web.NewPlugin(web.Config{Host: "127.0.0.1", Port: 0})
	err = h.Install(webPlugin)
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	client := &nethttp.Client{Timeout: 2 * time.Second}
	response, err := client.Get(webPlugin.URL() + "/chat")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	full := string(body)
	if !strings.Contains(full, "<!doctype html>") || !strings.Contains(full, "创建一个项目开始对话") {
		t.Fatalf("full page = %q", full)
	}
	if strings.Contains(full, "https://") || !strings.Contains(full, "/static/htmx.min.js") {
		t.Fatalf("full page does not use only local assets: %q", full)
	}
	if strings.Count(full, "<aside") != 1 || !strings.Contains(full, `id="product-navigation"`) {
		t.Fatalf("full page does not use one Web sidebar: %q", full)
	}

	request, err := nethttp.NewRequest(nethttp.MethodGet, webPlugin.URL()+"/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("HX-Request", "true")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	partial := string(body)
	if !strings.HasPrefix(partial, "<div id=\"product-navigation\" hx-swap-oob=\"outerHTML\"") ||
		!strings.Contains(partial, "<main id=\"main\"") ||
		strings.Contains(partial, "<!doctype html>") {
		t.Fatalf("HTMX partial = %q", partial)
	}
}

func TestCreateProjectAndCancel(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()

	workspace := t.TempDir()
	previous := selectWorkspace
	selectWorkspace = func(context.Context) (string, error) { return workspace, nil }
	t.Cleanup(func() { selectWorkspace = previous })

	client := &nethttp.Client{Timeout: 2 * time.Second}
	response, err := client.Post(webPlugin.URL()+"/chat/projects", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.Request.URL.Path == "/chat" || !strings.Contains(string(body), "会话已经创建") {
		t.Fatalf("create response path=%q body=%q", response.Request.URL.Path, body)
	}

	selectWorkspace = func(context.Context) (string, error) { return "", ErrCanceled }
	response, err = client.Post(webPlugin.URL()+"/chat/projects", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.Request.URL.Path != "/chat" {
		t.Fatalf("cancel response path = %q", response.Request.URL.Path)
	}
}

func TestCreateProjectFailureDoesNotCreateSession(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()

	previous := selectWorkspace
	selectWorkspace = func(context.Context) (string, error) { return "", errors.New("picker failed") }
	t.Cleanup(func() { selectWorkspace = previous })

	response, err := (&nethttp.Client{Timeout: 2 * time.Second}).Post(webPlugin.URL()+"/chat/projects", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != nethttp.StatusInternalServerError {
		t.Fatalf("status = %d", response.StatusCode)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	list, err := sessions.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("sessions = %#v", list)
	}
}

func TestSettingsFailureDiscardsNewSession(t *testing.T) {
	h := host.NewHost()
	err := h.Install(&persist.Plugin{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&session.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	previous := selectWorkspace
	selectWorkspace = func(context.Context) (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { selectWorkspace = previous })

	handler := &PageHandler{sessions: sessions, settings: failingSettings{}}
	request := httptest.NewRequest(nethttp.MethodPost, "/chat/projects", nil)
	request.Header.Set("HX-Request", "true")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != nethttp.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	list, err := sessions.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("sessions = %#v", list)
	}
}

type failingSettings struct{}

func (failingSettings) For(string) (settings.SessionSettings, error) {
	return settings.SessionSettings{}, errors.New("unavailable")
}

func (failingSettings) Put(string, settings.SessionSettings) error {
	return errors.New("disk full")
}

func installChat(t *testing.T) (*host.Host, *web.Plugin) {
	t.Helper()
	h := host.NewHost()
	err := h.Install(&persist.Plugin{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&session.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	webPlugin := web.NewPlugin(web.Config{Host: "127.0.0.1", Port: 0})
	err = h.Install(webPlugin)
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	return h, webPlugin
}
