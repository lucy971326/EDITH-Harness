//go:build windows

package machinelocal

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"github.com/charmbracelet/x/xpty"
	"golang.org/x/sys/windows"
)

type platformProcess struct {
	mu       sync.Mutex
	job      windows.Handle
	closed   bool
	assigned bool
}

func newPlatformProcess() (*platformProcess, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	return &platformProcess{job: job}, nil
}

func (p *platformProcess) prepare(_ *exec.Cmd) {}

func (p *platformProcess) started(cmd *exec.Cmd) error {
	p.mu.Lock()
	assigned := p.assigned
	p.mu.Unlock()
	if assigned {
		return nil
	}

	handle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return p.attach(handle)
}

func startTerminal(terminal xpty.Pty, cmd *exec.Cmd, platform *platformProcess) error {
	conPTY, ok := terminal.(*xpty.ConPty)
	if !ok {
		return fmt.Errorf("unsupported Windows terminal %T", terminal)
	}

	pid, rawHandle, err := conPTY.Spawn(cmd.Path, cmd.Args, &syscall.ProcAttr{
		Dir: cmd.Dir,
		Env: cmd.Env,
		Sys: cmd.SysProcAttr,
	})
	if err != nil {
		return err
	}
	handle := windows.Handle(rawHandle)
	defer windows.CloseHandle(handle)

	err = platform.attach(handle)
	if err != nil {
		_ = windows.TerminateProcess(handle, 1)
		return err
	}

	cmd.Process, err = os.FindProcess(pid)
	if err != nil {
		_ = platform.terminateJob(1)
		return fmt.Errorf("find terminal process after starting: %w", err)
	}
	return nil
}

func (p *platformProcess) attach(handle windows.Handle) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("process job is closed")
	}
	err := windows.AssignProcessToJobObject(p.job, handle)
	if err != nil {
		return err
	}
	p.assigned = true
	return nil
}

func (p *platformProcess) interrupt(_ *exec.Cmd) error {
	return p.terminateJob(130)
}

func (p *platformProcess) terminate(_ *exec.Cmd) error {
	return p.terminateJob(1)
}

func (p *platformProcess) close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return windows.CloseHandle(p.job)
}

func (p *platformProcess) terminateJob(exitCode uint32) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	return windows.TerminateJobObject(p.job, exitCode)
}
