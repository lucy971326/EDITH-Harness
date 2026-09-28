package desktop

import "testing"

func TestCloseAction(t *testing.T) {
	cases := []struct {
		name, want string
		state      closeState
	}{
		{name: "quit while idle", want: "quit"},
		{name: "quit during run", state: closeState{running: true}, want: "confirm"},
		{name: "quit with terminal", state: closeState{terminalOpen: true}, want: "confirm"},
		{name: "quit with unsaved changes", state: closeState{dirty: true}, want: "confirm"},
		{name: "quit while saving", state: closeState{saving: true}, want: "blocked"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := closeAction(item.state)
			if got != item.want {
				t.Fatalf("closeAction = %q, want %q", got, item.want)
			}
		})
	}
}
