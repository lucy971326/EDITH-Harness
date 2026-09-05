package builtin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"harness/kernel/machine"
	kernskills "harness/kernel/skills"
)

func TestProviderMaterializesSystemSkill(t *testing.T) {
	home := t.TempDir()
	provider, err := newProvider(testMachine{home: home}, []byte("skill content"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(provider.location)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "skill content" {
		t.Fatalf("materialized content = %q", data)
	}
	list, err := provider.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "skill-creator" || list[0].Scope != kernskills.ScopeSystem || list[0].Location != filepath.Join(home, skillCreatorLocation) {
		t.Fatalf("system skill = %#v", list)
	}
}

type testMachine struct {
	home string
}

var _ machine.Machine = testMachine{}

func (m testMachine) HomeDir() (string, error) { return m.home, nil }

func (testMachine) ReadFile(string) ([]byte, error) { return nil, errors.New("not implemented") }

func (testMachine) ReadDir(string) ([]machine.DirEntry, error) {
	return nil, errors.New("not implemented")
}

func (testMachine) WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (testMachine) ResolvePath(base, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Join(base, target)
}

func (testMachine) Run(context.Context, string, []string) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
