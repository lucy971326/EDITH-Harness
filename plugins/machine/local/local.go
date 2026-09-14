package machinelocal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// 活对象。挂在 Host 的 machine 键上的本机机器。
type local struct {
	bash string

	fileLocks pathLockSet

	mu        sync.Mutex
	watches   map[*localWatch]struct{}
	processes map[int64]*localProcess
	terminals map[*localProcess]struct{}
	closed    bool
}

func newLocal() (*local, error) {
	bash, err := findBash()
	if err != nil {
		return nil, err
	}
	return &local{
		bash:      bash,
		watches:   make(map[*localWatch]struct{}),
		processes: make(map[int64]*localProcess),
		terminals: make(map[*localProcess]struct{}),
	}, nil
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
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	watches := make([]*localWatch, 0, len(m.watches))
	for watch := range m.watches {
		watches = append(watches, watch)
	}
	processes := make([]*localProcess, 0, len(m.processes)+len(m.terminals))
	for _, process := range m.processes {
		processes = append(processes, process)
	}
	for process := range m.terminals {
		processes = append(processes, process)
	}
	m.mu.Unlock()

	var closeErrs []error
	for _, watch := range watches {
		err := watch.Close()
		if err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("machine-local: close watcher: %w", err))
		}
	}
	for _, process := range processes {
		err := process.terminate()
		if err != nil {
			closeErrs = append(closeErrs, err)
		}
	}
	deadline := time.Now().Add(processTerminationTimeout)
	for _, process := range processes {
		err := process.waitUntil(deadline)
		if err != nil {
			closeErrs = append(closeErrs, err)
		}
	}
	return errors.Join(closeErrs...)
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
