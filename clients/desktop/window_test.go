package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWindowPreferencesRecoverCorruptFile(t *testing.T) {
	root := t.TempDir()
	err := os.MkdirAll(filepath.Join(root, "desktop"), 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(root, "desktop", "window.json"), []byte("{"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := NewWindowPreferences(root)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.loaded || prefs.layout.Width != 1440 || prefs.layout.Height != 900 {
		t.Fatalf("corrupt preference should use centered defaults: %+v", prefs)
	}
}

func TestFitLayoutAfterDisplayRemoval(t *testing.T) {
	screens := []*application.Screen{{
		IsPrimary: true, WorkArea: application.Rect{X: 0, Y: 0, Width: 1600, Height: 900},
	}}
	saved := WindowLayout{X: 2100, Y: 100, Width: 1200, Height: 700, Maximized: true}
	got := fitLayout(saved, screens)
	if got.X != 200 || got.Y != 100 || got.Width != 1200 || got.Height != 700 || !got.Maximized {
		t.Fatalf("removed display should restore on primary work area: %+v", got)
	}
}
