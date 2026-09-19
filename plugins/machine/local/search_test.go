package machinelocal

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

)

func TestSearchPathsNestedIgnoreAndMatching(t *testing.T) {
	m := newTestLocal(t)
	root := t.TempDir()
	files := map[string]string{
		".gitignore":                  "node_modules/\n*.log\n/root-only.txt\n**/cache/\n\\#secret.txt\nignored/\n!ignored/keep.txt\n",
		".git/config":                 "",
		"node_modules/dependency.txt": "",
		"ignored/keep.txt":            "",
		"#secret.txt":                 "",
		"root-only.txt":               "",
		"root.log":                    "",
		"README.md":                   "",
		"src/.gitignore":              "!keep.log\nprivate?.txt\n",
		"src/keep.log":                "",
		"src/drop.log":                "",
		"src/private1.txt":            "",
		"src/main.go":                 "",
		"src/root-only.txt":           "",
		"src/cache/cached.txt":        "",
		"other/keep.log":              "",
		"other/private1.txt":          "",
		"other/main.go":               "",
		"中文/文件.txt":                   "",
	}
	for name, content := range files {
		err := m.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content))
		if err != nil {
			t.Fatal(err)
		}
	}
	err := os.Mkdir(filepath.Join(root, "empty"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		query string
		paths []string
	}{
		{"", []string{".gitignore", "README.md", "empty", "other", "src", "中文"}},
		{".txt", []string{"other/private1.txt", "src/root-only.txt", "中文/文件.txt"}},
		{"keep", []string{"src/keep.log"}},
		{"MAIN", []string{"other/main.go", "src/main.go"}},
		{"SRC\\MA", []string{"src/main.go"}},
		{"文件", []string{"中文/文件.txt"}},
		{"empty", []string{"empty"}},
		{"not-found", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			result, err := m.SearchPaths(t.Context(), root, tc.query)
			if err != nil {
				t.Fatal(err)
			}
			paths := make([]string, 0, len(result.Entries))
			for _, entry := range result.Entries {
				paths = append(paths, entry.Path)
				info, err := os.Stat(filepath.Join(root, filepath.FromSlash(entry.Path)))
				if err != nil {
					t.Fatal(err)
				}
				if (entry.Kind == "directory") != info.IsDir() {
					t.Fatalf("wrong kind: %+v", entry)
				}
			}
			if result.Truncated || !reflect.DeepEqual(paths, tc.paths) {
				t.Fatalf("search %q = %+v; want %v", tc.query, result, tc.paths)
			}
		})
	}
}

func TestSearchPathsLimit(t *testing.T) {
	m := newTestLocal(t)
	root := t.TempDir()
	for i := 0; i < 50; i++ {
		err := m.WriteFile(filepath.Join(root, fmt.Sprintf("file-%02d.txt", i)), nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := m.SearchPaths(t.Context(), root, "file")
	if err != nil || len(result.Entries) != 50 || result.Truncated {
		t.Fatalf("exact limit = %+v, %v", result, err)
	}
	err = m.WriteFile(filepath.Join(root, "file-50.txt"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "file"} {
		result, err = m.SearchPaths(t.Context(), root, query)
		if err != nil || len(result.Entries) != 50 || !result.Truncated {
			t.Fatalf("truncation = %+v, %v", result, err)
		}
	}
}
