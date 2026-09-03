package chat

import (
	"context"
	"errors"
	"io"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"harness/kernel/host"
)

func TestComposerActionRegistryRegistersAndSortsDefinitions(t *testing.T) {
	registry := newComposerActionRegistry()
	for _, definition := range []ComposerActionDefinition{
		{ID: "second", Order: 20},
		{ID: "first", Order: 10},
		{ID: "alpha", Order: 10},
	} {
		if err := registry.RegisterComposerAction(testComposerAction{definition: definition}); err != nil {
			t.Fatal(err)
		}
	}
	actions := registry.ComposerActions()
	if len(actions) != 3 || actions[0].ID != "alpha" || actions[1].ID != "first" || actions[2].ID != "second" {
		t.Fatalf("actions = %#v", actions)
	}
	if _, ok := registry.ComposerAction("first"); !ok {
		t.Fatal("first action is missing")
	}
}

func TestComposerActionRegistryRejectsInvalidAndDuplicateActions(t *testing.T) {
	registry := newComposerActionRegistry()
	if err := registry.RegisterComposerAction(nil); err == nil {
		t.Fatal("RegisterComposerAction(nil) succeeded")
	}
	for _, definition := range []ComposerActionDefinition{
		{ID: ""},
		{ID: "Upper"},
		{ID: "7starts"},
		{ID: "has space"},
	} {
		if err := registry.RegisterComposerAction(testComposerAction{definition: definition}); err == nil {
			t.Fatalf("RegisterComposerAction(%#v) succeeded", definition)
		}
	}
	valid := testComposerAction{definition: ComposerActionDefinition{ID: "demo", Order: 1}}
	if err := registry.RegisterComposerAction(valid); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterComposerAction(valid); err == nil {
		t.Fatal("RegisterComposerAction accepted duplicate")
	}
}

func TestPageHandlerRendersComposerActionsAndIsolatesErrors(t *testing.T) {
	h, webPlugin := installChat(t)
	defer h.Close()

	chatService, err := host.Resolve[Service](h, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if err := chatService.RegisterComposerAction(testComposerAction{
		definition: ComposerActionDefinition{ID: "btn1", Order: 10},
	}); err != nil {
		t.Fatal(err)
	}
	if err := chatService.RegisterComposerAction(errComposerAction{
		definition: ComposerActionDefinition{ID: "err-btn", Order: 20},
	}); err != nil {
		t.Fatal(err)
	}
	if err := chatService.RegisterComposerAction(nilComposerAction{
		definition: ComposerActionDefinition{ID: "nil-btn", Order: 30},
	}); err != nil {
		t.Fatal(err)
	}

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
	_ = response.Body.Close()

	html := string(body)
	if !strings.Contains(html, `id="composer-actions"`) {
		t.Fatalf("composer-actions container missing: %s", html)
	}
	if !strings.Contains(html, `<button type="button">demo</button>`) {
		t.Fatalf("action content missing: %s", html)
	}
	if strings.Contains(html, `err-btn`) {
		t.Fatalf("err-btn should be skipped: %s", html)
	}
	if strings.Contains(html, `nil-btn`) {
		t.Fatalf("nil-btn should be skipped: %s", html)
	}
}

type testComposerAction struct {
	definition ComposerActionDefinition
}

func (a testComposerAction) Definition() ComposerActionDefinition {
	return a.definition
}

func (testComposerAction) Render(ComposerActionContext) (templ.Component, error) {
	return templ.Raw("<button type=\"button\">demo</button>"), nil
}

type errComposerAction struct {
	definition ComposerActionDefinition
}

func (a errComposerAction) Definition() ComposerActionDefinition {
	return a.definition
}

func (errComposerAction) Render(ComposerActionContext) (templ.Component, error) {
	return nil, errors.New("render failed")
}

type nilComposerAction struct {
	definition ComposerActionDefinition
}

func (a nilComposerAction) Definition() ComposerActionDefinition {
	return a.definition
}

func (nilComposerAction) Render(ComposerActionContext) (templ.Component, error) {
	return nil, nil
}
