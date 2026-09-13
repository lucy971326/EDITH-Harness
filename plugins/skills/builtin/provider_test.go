package builtin

import (
	"os"
	"path/filepath"
	"testing"

	"harness/kernel/persist"
	kernskills "harness/kernel/skills"
)

func TestProviderMaterializesSystemSkill(t *testing.T) {
	home := t.TempDir()
	files, err := persist.NewFiles(filepath.Join(home, ".harness"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := newProvider(files, []byte("skill content"))
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
	if len(list) != 1 || list[0].Name != "skill-creator" || list[0].Scope != kernskills.ScopeSystem || list[0].Location != filepath.Join(home, ".harness", "system", "skills", "skill-creator", "SKILL.md") {
		t.Fatalf("system skill = %#v", list)
	}
}
