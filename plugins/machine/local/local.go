package machinelocal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// 活对象。挂在 Host 的 machine 键上的本机机器。
type local struct {
	bash string

	fileLocks pathLockSet

	watchMu sync.Mutex
	watches map[*localWatch]struct{}
	closed  bool
}

func newLocal() (*local, error) {
	bash, err := findBash()
	if err != nil {
		return nil, err
	}
	return &local{bash: bash, watches: make(map[*localWatch]struct{})}, nil
}

func (m *local) HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("machine-local: find home directory: %w", err)
	}
	return home, nil
}

func (m *local) ResolvePath(workspace string, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(workspace, path))
}

func (m *local) close() error {
	m.watchMu.Lock()
	m.closed = true
	watches := make([]*localWatch, 0, len(m.watches))
	for watch := range m.watches {
		watches = append(watches, watch)
	}
	m.watchMu.Unlock()

	var closeErr error
	for _, watch := range watches {
		err := watch.Close()
		if err != nil {
			closeErr = fmt.Errorf("machine-local: close watcher: %w", err)
		}
	}
	return closeErr
}

func (m *local) Run(ctx context.Context, dir string, argv []string) ([]byte, []byte, error) {
	if len(argv) == 0 {
		return nil, nil, fmt.Errorf("machine-local: empty argv")
	}

	program := argv[0]
	if program == "bash" {
		program = m.bash
	}

	cmd := exec.CommandContext(ctx, program, argv[1:]...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("machine-local: run %q: %w", argv[0], err)
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func findBash() (string, error) {
	for _, candidate := range bashCandidates() {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	name := "bash"
	if runtime.GOOS == "windows" {
		name = "bash.exe"
	}
	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}
	return "", fmt.Errorf("machine-local: bash not found")
}

func bashCandidates() []string {
	if runtime.GOOS != "windows" {
		return []string{"/bin/bash"}
	}

	var candidates []string
	for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		base := os.Getenv(variable)
		if base != "" {
			candidates = append(candidates, filepath.Join(base, "Git", "bin", "bash.exe"))
		}
	}
	return candidates
}
