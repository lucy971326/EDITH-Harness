package chat

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"harness/kernel/agents"
	"harness/kernel/host"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func TestUserMessageFromRequestAllowsImageOnly(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("images", "dot.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(tinyPNG); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("text", "   "); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(nethttp.MethodPost, "/chat/s/messages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(32 << 20); err != nil {
		t.Fatal(err)
	}
	got, err := userMessageFromRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 1 || got.Blocks[0].Kind != "image" || got.Blocks[0].Media == nil {
		t.Fatalf("message = %#v", got)
	}
	if got.Blocks[0].Media.MIME != "image/png" {
		t.Fatalf("mime = %q", got.Blocks[0].Media.MIME)
	}
	if got.Blocks[0].Media.Data != base64.StdEncoding.EncodeToString(tinyPNG) {
		t.Fatal("base64 mismatch")
	}
}

func TestUserMessageFromRequestRejectsEmpty(t *testing.T) {
	request := httptest.NewRequest(nethttp.MethodPost, "/chat/s/messages", strings.NewReader("text="))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := request.ParseForm(); err != nil {
		t.Fatal(err)
	}
	if _, err := userMessageFromRequest(request); err == nil {
		t.Fatal("want empty message error")
	}
}

func TestEnsureVisionRejectsImages(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()
	sessions, err := host.Resolve[*session.Store](h, "sessions")
	if err != nil {
		t.Fatal(err)
	}
	_, err = sessions.Create("session-1")
	if err != nil {
		t.Fatal(err)
	}
	settingsStore, err := host.Resolve[settings.SessionSettingsStore](h, "sessionSettings")
	if err != nil {
		t.Fatal(err)
	}
	err = settingsStore.Put("session-1", settings.SessionSettings{AgentID: agents.DefaultID, Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("images", "dot.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(tinyPNG); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"mode": "steer",
	} {
		err = form.WriteField(name, value)
		if err != nil {
			t.Fatal(err)
		}
	}
	err = form.Close()
	if err != nil {
		t.Fatal(err)
	}
	request, err := nethttp.NewRequest(nethttp.MethodPost, webPlugin.URL()+"/chat/session-1/messages", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := nethttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
