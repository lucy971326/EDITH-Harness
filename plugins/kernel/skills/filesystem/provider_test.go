package filesystem

import (
	"context"
	"errors"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"testing"

	"harness/kernel/machine"
	kernskills "harness/kernel/skills"
)

func TestProviderListsProjectAndUserRootsByPriority(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	writeSkill(t, filepath.Join(home, ".harness", "skills", "shared"), "shared", "user harness")
	writeSkill(t, filepath.Join(home, ".agents", "skills", "shared"), "shared", "user agents")
	writeSkill(t, filepath.Join(home, ".agents", "skills", "user-only"), "user-only", "user only")
	writeSkill(t, filepath.Join(workspace, ".agents", "skills", "shared"), "shared", "workspace agents")
	writeSkill(t, filepath.Join(workspace, ".harness", "skills", "shared"), "shared", "workspace harness")
	writeSkill(t, filepath.Join(workspace, ".harness", "skills", "project-only"), "project-only", "project only")

	provider := newProvider(testMachine{home: home})
	got, err := provider.List(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("List() = %#v, want three skills", got)
	}
	if got[0].Name != "project-only" || got[1].Name != "shared" || got[2].Name != "user-only" {
		t.Fatalf("List() order = %#v", got)
	}
	if got[1].Description != "workspace harness" || got[1].Scope != kernskills.ScopeWorkspace {
		t.Fatalf("shared skill = %#v", got[1])
	}
	if !filepath.IsAbs(got[0].Location) || got[0].Location != filepath.Join(workspace, ".harness", "skills", "project-only", "SKILL.md") {
		t.Fatalf("project location = %q", got[0].Location)
	}

	user, err := provider.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(user) != 2 || user[0].Name != "shared" || user[1].Name != "user-only" {
		t.Fatalf("user List() = %#v", user)
	}
	if user[0].Description != "user harness" || user[0].Scope != kernskills.ScopeUser {
		t.Fatalf("user shared skill = %#v", user[0])
	}
}

func TestProviderIgnoresMissingRootsAndNonDirectories(t *testing.T) {
	provider := newProvider(testMachine{home: t.TempDir()})
	got, err := provider.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("List() = %#v, want empty", got)
	}
}

func TestProviderRejectsBadCandidates(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing frontmatter", body: "# no frontmatter\n", want: "must start with YAML frontmatter"},
		{name: "missing description", body: "---\nname: good\n---\n", want: "description"},
		{name: "bad name", body: "---\nname: Bad\ndescription: valid\n---\n", want: "valid Agent Skill name"},
		{name: "mismatched name", body: "---\nname: other\ndescription: valid\n---\n", want: "does not match directory"},
		{name: "long description", body: "---\nname: good\ndescription: " + strings.Repeat("x", 1025) + "\n---\n", want: "1-1024"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			writeRawSkill(t, filepath.Join(home, ".harness", "skills", "good", "SKILL.md"), tt.body)
			_, err := newProvider(testMachine{home: home}).List("")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("List() error = %v, want %q", err, tt.want)
			}
		})
	}

	t.Run("missing SKILL.md", func(t *testing.T) {
		home := t.TempDir()
		err := os.MkdirAll(filepath.Join(home, ".harness", "skills", "empty"), 0o755)
		if err != nil {
			t.Fatal(err)
		}
		got, err := newProvider(testMachine{home: home}).List("")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("List() = %#v, want empty", got)
		}
	})
}

func TestProviderAllowsUnknownFrontmatter(t *testing.T) {
	home := t.TempDir()
	writeRawSkill(t, filepath.Join(home, ".harness", "skills", "good", "SKILL.md"), "---\nname: good\ndescription: valid\nmetadata:\n  author: test\ncustom: [one, two]\n---\n# Instructions\n")
	got, err := newProvider(testMachine{home: home}).List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Description != "valid" {
		t.Fatalf("List() = %#v", got)
	}
}

func TestProviderUsesCurrentMachinePaths(t *testing.T) {
	const (
		home      = "/machine/home"
		workspace = "/machine/work"
		skillRoot = "/machine/work/.agents/skills"
		skillFile = skillRoot + "/remote-skill/SKILL.md"
	)
	m := memoryMachine{
		home: home,
		directories: map[string][]machine.DirEntry{
			skillRoot: {{Name: "remote-skill", IsDir: true}},
		},
		files: map[string][]byte{
			skillFile: []byte("---\nname: remote-skill\ndescription: Remote Skill.\n---\nInstructions.\n"),
		},
	}
	got, err := newProvider(m).List(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Location != skillFile || got[0].Scope != kernskills.ScopeWorkspace {
		t.Fatalf("List() = %#v", got)
	}
}

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	writeRawSkill(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+name+"\ndescription: "+description+"\n---\n# Instructions\n")
}

func writeRawSkill(t *testing.T, path, body string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(path, []byte(body), 0o644)
	if err != nil {
		t.Fatal(err)
	}
}

type testMachine struct {
	home string
}

var _ machine.Machine = testMachine{}

func (m testMachine) HomeDir() (string, error) { return m.home, nil }

func (testMachine) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (testMachine) ReadDir(path string) ([]machine.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]machine.DirEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, machine.DirEntry{Name: entry.Name(), IsDir: entry.IsDir()})
	}
	return out, nil
}

func (testMachine) WriteFile(path string, data []byte) error { return os.WriteFile(path, data, 0o644) }

func (testMachine) ResolvePath(workspace, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(workspace, path)
}

func (testMachine) Run(_ context.Context, _ string, _ []string) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}

type memoryMachine struct {
	home        string
	directories map[string][]machine.DirEntry
	files       map[string][]byte
}

func (m memoryMachine) HomeDir() (string, error) { return m.home, nil }

func (m memoryMachine) ReadFile(path string) ([]byte, error) {
	data, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (m memoryMachine) ReadDir(path string) ([]machine.DirEntry, error) {
	entries, ok := m.directories[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]machine.DirEntry(nil), entries...), nil
}

func (memoryMachine) WriteFile(string, []byte) error { return errors.New("not implemented") }

func (memoryMachine) ResolvePath(base, target string) string {
	if pathpkg.IsAbs(target) {
		return pathpkg.Clean(target)
	}
	return pathpkg.Join(base, target)
}

func (memoryMachine) Run(context.Context, string, []string) ([]byte, []byte, error) {
	return nil, nil, errors.New("not implemented")
}
