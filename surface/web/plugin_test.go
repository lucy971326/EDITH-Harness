package web

import (
	"io"
	"net"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"harness/kernel/host"
)

func TestPluginStartsServesAssetsAndCloses(t *testing.T) {
	h := host.NewHost()
	plugin := NewPlugin(Config{Host: loopbackHost, Port: 0})
	err := h.Install(plugin)
	if err != nil {
		t.Fatal(err)
	}
	service, err := host.Resolve[Service](h, "web")
	if err != nil {
		t.Fatal(err)
	}
	err = service.RegisterProduct(Product{ID: "test", Name: "测试", BasePath: "/test"})
	if err != nil {
		t.Fatal(err)
	}
	err = service.RegisterRoute("GET /test", &textHandler{text: "ready"})
	if err != nil {
		t.Fatal(err)
	}

	client := &nethttp.Client{Timeout: 2 * time.Second}
	response, err := client.Get(plugin.URL())
	if err != nil {
		t.Fatal(err)
	}
	if response.Request.URL.Path != "/test" {
		t.Fatalf("root redirected to %q", response.Request.URL.Path)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	response, err = client.Get(plugin.URL() + "/static/htmx.min.js")
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
	if response.StatusCode != nethttp.StatusOK || !strings.Contains(string(body), "htmx") {
		t.Fatalf("static htmx = (%d, %d bytes)", response.StatusCode, len(body))
	}

	url := plugin.URL()
	err = h.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(url + "/test")
	if err == nil {
		t.Fatal("request after Close error = nil")
	}
}

func TestPluginRejectsInvalidConfig(t *testing.T) {
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

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	h := host.NewHost()
	err = h.Install(NewPlugin(Config{Host: loopbackHost, Port: port}))
	if err == nil {
		t.Fatal("Install on occupied port error = nil")
	}
}
