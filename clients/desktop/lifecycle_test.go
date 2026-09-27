package desktop

import "testing"

func TestCloseAction(t *testing.T) {
	cases := []struct {
		name, mode, want string
		state            closeState
		tray             bool
	}{
		{name: "idle", mode: "window", tray: true, want: "quit"},
		{name: "running", mode: "window", tray: true, state: closeState{running: true}, want: "hide"},
		{name: "terminal", mode: "window", tray: true, state: closeState{terminalOpen: true}, want: "hide"},
		{name: "dirty", mode: "window", tray: true, state: closeState{dirty: true}, want: "confirm"},
		{name: "saving", mode: "window", tray: true, state: closeState{saving: true}, want: "blocked"},
		{name: "background while saving", mode: "window", tray: true, state: closeState{running: true, saving: true}, want: "hide"},
		{name: "tray quit during run", mode: "quit", tray: true, state: closeState{running: true}, want: "confirm"},
		{name: "linux running", mode: "window", state: closeState{running: true}, want: "native-confirm"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := closeAction(item.mode, item.state, item.tray)
			if got != item.want {
				t.Fatalf("closeAction = %q, want %q", got, item.want)
			}
		})
	}
}
