package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harness.yaml")
	err := os.WriteFile(path, []byte("web:\n  host: 127.0.0.1\n  port: 3210\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Web.Host != "127.0.0.1" || config.Web.Port != 3210 {
		t.Fatalf("loadConfig() = %+v", config)
	}
}

func TestLoadConfigRejectsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harness.yaml")
	err := os.WriteFile(path, []byte("web: ["), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadConfig(path)
	if err == nil {
		t.Fatal("loadConfig(invalid) error = nil")
	}
}

func TestLoadConfigRejectsRemovedDataDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harness.yaml")
	err := os.WriteFile(path, []byte("dataDir: .harness-data\nweb:\n  host: 127.0.0.1\n  port: 3210\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadConfig(path)
	if err == nil {
		t.Fatal("loadConfig(dataDir) error = nil")
	}
}

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
