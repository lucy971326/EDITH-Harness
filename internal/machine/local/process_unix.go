//go:build !windows

package machinelocal

import (
	"errors"
	"os/exec"
	"syscall"

	"github.com/charmbracelet/x/xpty"
)

type platformProcess struct {
	processGroupID int
}

func newPlatformProcess() (*platformProcess, error) {
	return &platformProcess{}, nil
}

func (p *platformProcess) prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func (p *platformProcess) started(cmd *exec.Cmd) error {
	p.processGroupID = cmd.Process.Pid
	return nil
}

func startTerminal(terminal xpty.Pty, cmd *exec.Cmd, _ *platformProcess) error {
	return terminal.Start(cmd)
}

func (p *platformProcess) interrupt(_ *exec.Cmd) error {
	err := syscall.Kill(-p.processGroupID, syscall.SIGINT)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (p *platformProcess) terminate(_ *exec.Cmd) error {
	err := syscall.Kill(-p.processGroupID, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (p *platformProcess) close() error {
	return p.terminate(nil)
}
