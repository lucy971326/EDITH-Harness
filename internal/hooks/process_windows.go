//go:build windows

package hooks

import (
	"io"
	"os/exec"
	"strconv"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		stop := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		stop.Stdout = io.Discard
		stop.Stderr = io.Discard
		if err := stop.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
