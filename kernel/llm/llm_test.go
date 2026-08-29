package llm

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"harness/kernel/host"
	"harness/kernel/kinds"
	"harness/kernel/session"
)

type fakeAdapter struct {
	calls  []Call
	chunks []Chunk
	err    error
}

func (a *fakeAdapter) Stream(_ context.Context, call Call) (<-chan Chunk, error) {
	a.calls = append(a.calls, call)
	if a.err != nil {
		return nil, a.err
	}
	out := make(chan Chunk, len(a.chunks))
	for _, chunk := range a.chunks {
		out <- chunk
	}
	close(out)
	return out, nil
}

func TestPluginRegistersService(t *testing.T) {
	h := host.New()
	plugin := &Plugin{
		Config:   testConfig(),
		Catalog:  testCatalog(),
		Adapters: map[string]Adapter{"completions": doneAdapter()},
	}
	if err := h.Install(plugin); err != nil {
		t.Fatalf("Install: %v", err)
	}
	service, err := host.Resolve[*Service](h, "llm")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if service != plugin.service {
		t.Fatal("resolved service differs from plugin service")
	}
}

func TestStreamResolvesModelAPIAndReasoning(t *testing.T) {
	completions := doneAdapter()
	responses := doneAdapter()
	service := newTestService(t, map[string]Adapter{
		"completions": completions,
		"responses":   responses,
	})

	stream, err := service.Stream(context.Background(), kinds.Setup{
		Model:           "test/chat",
		ReasoningEffort: "high",
	}, Request{System: "system", Messages: []session.Message{{Role: session.RoleUser, Blocks: []session.Block{{Kind: "text", Text: "hello"}}}}})
	if err != nil {
		t.Fatalf("Stream chat: %v", err)
	}
	assertDone(t, collect(stream))
	if len(completions.calls) != 1 {
		t.Fatalf("completions calls = %d, want 1", len(completions.calls))
	}
	call := completions.calls[0]
	if call.Target.API != "completions" || call.Target.RemoteID != "upstream-chat" {
		t.Fatalf("target = %#v", call.Target)
	}
	wantReasoning := Patch{
		"thinking":         map[string]any{"type": "enabled"},
		"reasoning_effort": "high",
	}
	if !reflect.DeepEqual(call.Reasoning, wantReasoning) {
		t.Fatalf("reasoning = %#v, want %#v", call.Reasoning, wantReasoning)
	}

	stream, err = service.Stream(context.Background(), kinds.Setup{Model: "test/vision"}, Request{})
	if err != nil {
		t.Fatalf("Stream vision: %v", err)
	}
	assertDone(t, collect(stream))
	if len(responses.calls) != 1 {
		t.Fatalf("responses calls = %d, want 1", len(responses.calls))
	}
	if got := responses.calls[0].Target; got.API != "responses" || got.RemoteID != "upstream-vision" {
		t.Fatalf("override target = %#v", got)
	}
}

func TestStreamRejectsInvalidSelection(t *testing.T) {
	service := newTestService(t, map[string]Adapter{"completions": doneAdapter()})

	for _, test := range []struct {
		name  string
		setup kinds.Setup
		want  string
	}{
		{"bad model key", kinds.Setup{Model: "chat"}, "provider/model"},
		{"unknown provider", kinds.Setup{Model: "other/chat"}, "not configured"},
		{"disabled model", kinds.Setup{Model: "test/missing"}, "not enabled"},
		{"uncompiled API", kinds.Setup{Model: "test/vision"}, "not compiled"},
		{"unsupported effort", kinds.Setup{Model: "test/chat", ReasoningEffort: "max"}, "does not support"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.Stream(context.Background(), test.setup, Request{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Stream error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStreamProjectsImagesWithoutMutatingHistory(t *testing.T) {
	adapter := doneAdapter()
	service := newTestService(t, map[string]Adapter{"completions": adapter, "responses": adapter})
	messages := []session.Message{{
		Role: session.RoleUser,
		Blocks: []session.Block{
			{Kind: "image", Media: &session.Media{MIME: "image/png", Data: "base64-image"}},
			{Kind: "text", Text: "keep"},
		},
	}}

	stream, err := service.Stream(context.Background(), kinds.Setup{Model: "test/chat"}, Request{Messages: messages})
	if err != nil {
		t.Fatalf("Stream no-vision: %v", err)
	}
	assertDone(t, collect(stream))
	call := adapter.calls[0]
	if got := call.Messages[0].Blocks[0]; got.Kind != "text" || got.Text != imageUnsupportedText || got.Media != nil {
		t.Fatalf("projected image = %#v", got)
	}
	if got := messages[0].Blocks[0]; got.Kind != "image" || got.Media == nil || got.Media.Data != "base64-image" {
		t.Fatalf("original history changed: %#v", got)
	}

	stream, err = service.Stream(context.Background(), kinds.Setup{Model: "test/vision"}, Request{Messages: messages})
	if err != nil {
		t.Fatalf("Stream vision: %v", err)
	}
	assertDone(t, collect(stream))
	if got := adapter.calls[1].Messages[0].Blocks[0]; got.Kind != "image" || got.Media == nil || got.Media.Data != "base64-image" {
		t.Fatalf("vision image = %#v", got)
	}
}

func TestStreamPreservesInterleavingAndEnforcesTerminal(t *testing.T) {
	adapter := &fakeAdapter{chunks: []Chunk{
		{Index: 1, Kind: "text", Delta: "A"},
		{Index: 0, Kind: "reasoning", Delta: "B"},
		{Done: true},
	}}
	service := newTestService(t, map[string]Adapter{"completions": adapter})
	stream, err := service.Stream(context.Background(), kinds.Setup{Model: "test/chat"}, Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	chunks := collect(stream)
	if len(chunks) != 3 || chunks[0].Index != 1 || chunks[1].Index != 0 {
		t.Fatalf("interleaved chunks = %#v", chunks)
	}
	assertDone(t, chunks)

	bad := &fakeAdapter{chunks: []Chunk{{Done: true, Err: errors.New("both")}}}
	service = newTestService(t, map[string]Adapter{"completions": bad})
	stream, err = service.Stream(context.Background(), kinds.Setup{Model: "test/chat"}, Request{})
	if err != nil {
		t.Fatalf("Stream bad terminal: %v", err)
	}
	chunks = collect(stream)
	if len(chunks) != 1 || chunks[0].Err == nil || chunks[0].Done {
		t.Fatalf("bad terminal result = %#v", chunks)
	}
}

func testConfig() Config {
	return Config{Providers: map[string]ProviderConfig{
		"test": {
			Catalog: "facts",
			API:     "completions",
			Models: map[string]ModelConfig{
				"chat":   {ID: "upstream-chat"},
				"vision": {ID: "upstream-vision", API: "responses"},
			},
		},
	}}
}

func testCatalog() Catalog {
	return Catalog{Providers: map[string]CatalogProvider{
		"facts": {Models: map[string]CatalogModel{
			"chat": {
				Reasoning: map[string]Patch{
					"high": {
						"thinking":         map[string]any{"type": "enabled"},
						"reasoning_effort": "high",
					},
				},
			},
			"vision": {Vision: true},
		}},
	}}
}

func doneAdapter() *fakeAdapter {
	return &fakeAdapter{chunks: []Chunk{{Done: true}}}
}

func newTestService(t *testing.T, adapters map[string]Adapter) *Service {
	t.Helper()
	service, err := New(testConfig(), testCatalog(), adapters)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return service
}

func collect(stream <-chan Chunk) []Chunk {
	var chunks []Chunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func assertDone(t *testing.T, chunks []Chunk) {
	t.Helper()
	if len(chunks) == 0 || !chunks[len(chunks)-1].Done || chunks[len(chunks)-1].Err != nil {
		t.Fatalf("terminal chunks = %#v, want final Done", chunks)
	}
}
