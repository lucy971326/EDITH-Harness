//go:build darwin

package chat

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func chooseWorkspace(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, "osascript", "-e", `POSIX path of (choose folder with prompt "选择工作区目录")`)
	body, err := command.Output()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && strings.Contains(string(exitError.Stderr), "User canceled") {
			return "", ErrCanceled
		}
		return "", fmt.Errorf("run osascript: %w", err)
	}
	path := strings.TrimSpace(string(body))
	if path == "" {
		return "", ErrCanceled
	}
	return path, nil
}
