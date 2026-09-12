package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserDataDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := userDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".harness"); dir != want {
		t.Fatalf("userDataDir() = %q, want %q", dir, want)
	}
}
