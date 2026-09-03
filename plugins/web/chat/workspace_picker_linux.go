//go:build linux

package chat

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func chooseWorkspace(ctx context.Context) (string, error) {
	for _, picker := range []struct {
		name string
		args []string
	}{
		{name: "zenity", args: []string{"--file-selection", "--directory", "--title=选择工作区目录"}},
		{name: "kdialog", args: []string{"--getexistingdirectory", ".", "--title", "选择工作区目录"}},
	} {
		_, err := exec.LookPath(picker.name)
		if err != nil {
			continue
		}
		command := exec.CommandContext(ctx, picker.name, picker.args...)
		body, err := command.Output()
		if err != nil {
			var exitError *exec.ExitError
			if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
				return "", ErrCanceled
			}
			return "", fmt.Errorf("run %s: %w", picker.name, err)
		}
		path := strings.TrimSpace(string(body))
		if path == "" {
			return "", ErrCanceled
		}
		return path, nil
	}
	return "", fmt.Errorf("未找到 zenity 或 kdialog")
}
