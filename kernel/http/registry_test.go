package http

import (
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"harness/kernel/host"
)

// 活对象。测试使用的固定响应 Handler。
type textHandler struct {
	status int
	text   string
}

func (h *textHandler) ServeHTTP(w nethttp.ResponseWriter, _ *nethttp.Request) {
	w.WriteHeader(h.status)
	_, _ = io.WriteString(w, h.text)
}

// 活对象。测试使用的路径参数 Handler。
type pathValueHandler struct{}

func (*pathValueHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	_, _ = io.WriteString(w, r.PathValue("id"))
}

func TestRegistry_registersAndDispatches(t *testing.T) {
	registry := newRegistry()
	err := registry.Register("GET /chat/{id}", &pathValueHandler{})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(nethttp.MethodGet, "/chat/abc", nil)
	response := httptest.NewRecorder()
	registry.ServeHTTP(response, request)
	if response.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, nethttp.StatusOK)
	}
	if response.Body.String() != "abc" {
		t.Fatalf("body = %q, want %q", response.Body.String(), "abc")
	}

	request = httptest.NewRequest(nethttp.MethodPost, "/chat/abc", nil)
	response = httptest.NewRecorder()
	registry.ServeHTTP(response, request)
	if response.Code != nethttp.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", response.Code, nethttp.StatusMethodNotAllowed)
	}
}

func TestRegistry_rejectsInvalidAndConflictingPatterns(t *testing.T) {
	registry := newRegistry()
	err := registry.Register("GET /{value}", &textHandler{status: nethttp.StatusOK, text: "first"})
	if err != nil {
		t.Fatal(err)
	}

	err = registry.Register("GET /{", &textHandler{status: nethttp.StatusOK})
	if err == nil {
		t.Fatal("Register(invalid) error = nil")
	}
	err = registry.Register("GET /{value}", &textHandler{status: nethttp.StatusOK})
	if err == nil {
		t.Fatal("Register(duplicate) error = nil")
	}
	err = registry.Register("/fixed", &textHandler{status: nethttp.StatusOK})
	if err == nil {
		t.Fatal("Register(conflict) error = nil")
	}

	request := httptest.NewRequest(nethttp.MethodGet, "/kept", nil)
	response := httptest.NewRecorder()
	registry.ServeHTTP(response, request)
	if response.Code != nethttp.StatusOK || response.Body.String() != "first" {
		t.Fatalf("existing route after failed register = (%d, %q)", response.Code, response.Body.String())
	}
}

func TestRegistry_registerValidation(t *testing.T) {
	registry := newRegistry()
	err := registry.Register("", &textHandler{status: nethttp.StatusOK})
	if err == nil {
		t.Fatal("Register(empty) error = nil")
	}
	err = registry.Register("GET /", nil)
	if err == nil {
		t.Fatal("Register(nil) error = nil")
	}
}

func TestRegistry_concurrentServeAndRegistration(t *testing.T) {
	registry := newRegistry()
	err := registry.Register("GET /stable", &textHandler{status: nethttp.StatusOK, text: "stable"})
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go serveRepeatedly(registry, &wait)
	}
	for i := 0; i < 100; i++ {
		pattern := fmt.Sprintf("GET /temporary/%d", i)
		registerErr := registry.Register(pattern, &textHandler{status: nethttp.StatusOK})
		if registerErr != nil {
			t.Fatal(registerErr)
		}
	}
	wait.Wait()
}

func TestPlugin_startAndClose(t *testing.T) {
	h := host.NewHost()
	plugin := NewPlugin(Config{Host: loopbackHost, Port: 0})
	err := h.Install(plugin)
	if err != nil {
		t.Fatal(err)
	}

	registry, err := host.Resolve[*Registry](h, "http")
	if err != nil {
		t.Fatal(err)
	}
	err = registry.Register("GET /ready", &textHandler{status: nethttp.StatusOK, text: "ready"})
	if err != nil {
		t.Fatal(err)
	}

	url := "http://" + plugin.server.Addr + "/ready"
	client := &nethttp.Client{Timeout: 2 * time.Second}
	response, err := client.Get(url)
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
	if response.StatusCode != nethttp.StatusOK || string(body) != "ready" {
		t.Fatalf("GET /ready = (%d, %q)", response.StatusCode, string(body))
	}

	err = h.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(url)
	if err == nil {
		t.Fatal("GET after Close error = nil")
	}
}

func TestPlugin_rejectsConfigAndOccupiedPort(t *testing.T) {
	tests := []Config{
		{Host: "localhost", Port: 0},
		{Host: loopbackHost, Port: -1},
		{Host: loopbackHost, Port: 65536},
	}
	for _, config := range tests {
		h := host.NewHost()
		err := h.Install(NewPlugin(config))
		if err == nil {
			t.Fatalf("Install(%+v) error = nil", config)
		}
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	h := host.NewHost()
	err = h.Install(NewPlugin(Config{Host: loopbackHost, Port: port}))
	if err == nil {
		t.Fatal("Install(occupied port) error = nil")
	}
}

func serveRepeatedly(registry *Registry, wait *sync.WaitGroup) {
	defer wait.Done()
	for i := 0; i < 200; i++ {
		request := httptest.NewRequest(nethttp.MethodGet, "/stable", nil)
		response := httptest.NewRecorder()
		registry.ServeHTTP(response, request)
	}
}
