package machinelocal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"harness/kernel/machine"
)

var _ machine.Machine = (*local)(nil)

// 活对象。挂在 Host 的 machine 键上的本机机器。
type local struct {
	bash string
}

func newLocal() (*local, error) {
	bash, err := findBash()
	if err != nil {
		return nil, err
	}
	return &local{bash: bash}, nil
}

func (m *local) ReadFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("machine-local: read %q: %w", path, err)
	}
	return b, nil
}

func (m *local) WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return fmt.Errorf("machine-local: create parent for %q: %w", path, err)
	}

	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return fmt.Errorf("machine-local: write %q: %w", path, err)
	}
	return nil
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
