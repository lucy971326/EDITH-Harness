package chat

import (
	"io"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"harness/kernel/host"
	"harness/surface/web"
)

func TestPluginRendersFullPageAndHTMXMain(t *testing.T) {
	h := host.NewHost()
	webPlugin := web.NewPlugin(web.Config{Host: "127.0.0.1", Port: 0})
	err := h.Install(webPlugin)
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
	if !strings.Contains(full, "<!doctype html>") || !strings.Contains(full, "Web 产品壳已经就位") {
		t.Fatalf("full page = %q", full)
	}
	if strings.Contains(full, "https://") || !strings.Contains(full, "/static/htmx.min.js") {
		t.Fatalf("full page does not use only local assets: %q", full)
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
	if !strings.HasPrefix(partial, "<main id=\"main\"") || strings.Contains(partial, "<!doctype html>") {
		t.Fatalf("HTMX partial = %q", partial)
	}
}
