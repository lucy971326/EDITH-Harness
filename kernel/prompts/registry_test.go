package prompts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"harness/kernel/host"
	"harness/kernel/kinds"
)

func TestRegistry_assemblesRegisteredAndLocalParts(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Register(Part{
		Name:  "style",
		Order: 20,
		Render: func(_ context.Context, setup kinds.Setup) (string, error) {
			return "Style for " + setup.Kind, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Register(textPart("identity", 10, "  Identity  "))
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Assemble(context.Background(), kinds.Setup{
		Kind:      "llm",
		Persona:   "Go architect",
		Workspace: "/workspace",
	},
		Part{
			Name:  "persona",
			Order: 30,
			Render: func(_ context.Context, setup kinds.Setup) (string, error) {
				return setup.Persona, nil
			},
		},
		textPart("empty", 40, "  "),
		Part{
			Name:  "workspace",
			Order: 50,
			Render: func(_ context.Context, setup kinds.Setup) (string, error) {
				return setup.Workspace, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "Identity\n\nStyle for llm\n\nGo architect\n\n/workspace"
	if result != want {
		t.Fatalf("Assemble() = %q, want %q", result, want)
	}
}

func TestRegistry_preservesOrderForEqualOrder(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Register(textPart("first", 10, "first"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Register(textPart("second", 10, "second"))
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Assemble(
		context.Background(),
		kinds.Setup{},
		textPart("third", 10, "third"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result != "first\n\nsecond\n\nthird" {
		t.Fatalf("Assemble() = %q", result)
	}
}

func TestRegistry_rejectsInvalidAndDuplicateParts(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Register(Part{})
	if err == nil {
		t.Fatal("Register() error = nil, want empty name error")
	}
	_, err = registry.Register(Part{Name: "missing-render"})
	if err == nil {
		t.Fatal("Register() error = nil, want missing render error")
	}
	_, err = registry.Register(textPart("same", 10, "registered"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Register(textPart("same", 20, "duplicate"))
	if err == nil {
		t.Fatal("Register() error = nil, want duplicate error")
	}

	_, err = registry.Assemble(
		context.Background(),
		kinds.Setup{},
		textPart("same", 30, "local"),
	)
	if err == nil {
		t.Fatal("Assemble() error = nil, want duplicate error")
	}
	_, err = registry.Assemble(
		context.Background(),
		kinds.Setup{},
		textPart("local", 10, "one"),
		textPart("local", 20, "two"),
	)
	if err == nil {
		t.Fatal("Assemble() error = nil, want duplicate local error")
	}
}

func TestRegistry_returnsRenderErrorAndCancellation(t *testing.T) {
	registry := NewRegistry()
	wantErr := errors.New("failed")
	_, err := registry.Register(Part{
		Name:  "broken",
		Order: 10,
		Render: func(context.Context, kinds.Setup) (string, error) {
			return "", wantErr
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = registry.Assemble(context.Background(), kinds.Setup{})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("Assemble() error = %v, want wrapped render error", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = registry.Assemble(ctx, kinds.Setup{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Assemble() error = %v, want context canceled", err)
	}
}

func TestRegistry_unregisterIsIdempotent(t *testing.T) {
	registry := NewRegistry()
	unregister, err := registry.Register(textPart("part", 10, "text"))
	if err != nil {
		t.Fatal(err)
	}
	unregister()
	unregister()

	result, err := registry.Assemble(context.Background(), kinds.Setup{})
	if err != nil {
		t.Fatal(err)
	}
	if result != "" {
		t.Fatalf("Assemble() = %q, want empty", result)
	}
}

func TestPlugin_registersPromptsService(t *testing.T) {
	h := host.NewHost()
	err := h.Install(NewPlugin())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	service, err := host.Resolve[Prompts](h, "prompts")
	if err != nil {
		t.Fatal(err)
	}
	if service == nil {
		t.Fatal("prompts service = nil")
	}
}

func textPart(name string, order int, text string) Part {
	return Part{
		Name:  name,
		Order: order,
		Render: func(context.Context, kinds.Setup) (string, error) {
			return text, nil
		},
	}
}
