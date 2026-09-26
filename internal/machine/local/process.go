package machinelocal

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"harness/internal/machine"
	"harness/internal/permissions"

	"github.com/charmbracelet/x/xpty"
)

const (
	maxTrackedProcesses       = 64
	maxProcessOutput          = 1024 * 1024
	maxSafeProcessID          = int64(1<<53 - 1)
	processTerminationTimeout = 5 * time.Second
)

type localProcess struct {
	ownerID string
	cmd     *exec.Cmd
	pty     xpty.Pty
	process *platformProcess

	mu         sync.Mutex
	output     headTailBuffer
	exited     bool
	exitCode   int
	finishedAt time.Time

	interactionMu sync.Mutex
	done          chan struct{}
	readerDone    chan struct{}
	outputEvents  chan []byte

	terminateMu     sync.Mutex
	terminationSent bool
	cleanup         func()
}

func (m *Local) StartTerminal(request machine.TerminalRequest) (machine.TerminalProcess, error) {
	if request.Dir == "" || len(request.Argv) == 0 || request.Rows <= 0 || request.Cols <= 0 {
		return nil, fmt.Errorf("machine-local: terminal directory, argv and positive size required")
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("machine-local: machine is closed")
	}
	process, err := m.startProcess(machine.ProcessRequest{
		Dir:  request.Dir,
		Argv: request.Argv,
		TTY:  true,
	}, request.Cols, request.Rows, true)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.terminals[process] = struct{}{}
	m.mu.Unlock()

	go func() {
		<-process.done
		m.mu.Lock()
		delete(m.terminals, process)
		m.mu.Unlock()
	}()
	return process, nil
}

func (m *Local) AgentExec(ctx context.Context, policy permissions.Policy, request machine.ProcessRequest) (machine.ProcessOutput, error) {
	err := ctx.Err()
	if err != nil {
		return machine.ProcessOutput{}, err
	}
	if request.OwnerID == "" || len(request.Argv) == 0 || request.Wait < 0 {
		return machine.ProcessOutput{}, fmt.Errorf("machine-local: process owner, argv and non-negative wait required")
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return machine.ProcessOutput{}, fmt.Errorf("machine-local: machine is closed")
	}
	if len(m.processes) >= maxTrackedProcesses {
		m.pruneExitedProcessLocked()
	}
	if len(m.processes) >= maxTrackedProcesses {
		m.mu.Unlock()
		return machine.ProcessOutput{}, fmt.Errorf("machine-local: process limit of %d reached", maxTrackedProcesses)
	}
	processID, err := m.newProcessIDLocked()
	if err != nil {
		m.mu.Unlock()
		return machine.ProcessOutput{}, err
	}
	launch, err := m.prepareAgentLaunch(policy, request)
	if err != nil {
		m.mu.Unlock()
		return machine.ProcessOutput{}, err
	}
	err = ctx.Err()
	if err != nil {
		launch.close()
		m.mu.Unlock()
		return machine.ProcessOutput{}, err
	}
	process, err := m.startPreparedProcess(request, launch.cmd, 80, 24, false, launch.close)
	if err != nil {
		launch.close()
		m.mu.Unlock()
		return machine.ProcessOutput{}, err
	}
	m.processes[processID] = process
	m.mu.Unlock()

	output, err := process.wait(ctx, request.Wait, processID)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		terminateErr := process.terminate()
		waitErr := process.waitUntil(time.Now().Add(processTerminationTimeout))
		if waitErr == nil {
			m.removeProcess(processID, process)
		}
		return machine.ProcessOutput{}, errors.Join(err, terminateErr, waitErr)
	}
	if output.Exited {
		m.removeProcess(processID, process)
	}
	return output, nil
}

func (m *Local) AgentInteract(ctx context.Context, interaction machine.ProcessInteraction) (machine.ProcessOutput, error) {
	err := ctx.Err()
	if err != nil {
		return machine.ProcessOutput{}, err
	}
	if interaction.OwnerID == "" || interaction.ProcessID <= 0 || interaction.Wait < 0 {
		return machine.ProcessOutput{}, fmt.Errorf("machine-local: process owner, ID and non-negative wait required")
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return machine.ProcessOutput{}, fmt.Errorf("machine-local: machine is closed")
	}
	process := m.processes[interaction.ProcessID]
	if process == nil || process.ownerID != interaction.OwnerID {
		m.mu.Unlock()
		return machine.ProcessOutput{}, processNotFound(interaction.ProcessID)
	}
	m.mu.Unlock()

	process.interactionMu.Lock()
	defer process.interactionMu.Unlock()

	if len(interaction.Chars) > 0 && !process.hasExited() {
		var err error
		if process.pty != nil {
			_, err = process.pty.Write(interaction.Chars)
		} else if len(interaction.Chars) == 1 && interaction.Chars[0] == 3 {
			err = process.process.interrupt(process.cmd)
		} else {
			err = fmt.Errorf("machine-local: process %d has no terminal input", interaction.ProcessID)
		}
		if err != nil {
			return machine.ProcessOutput{}, fmt.Errorf("machine-local: interact with process %d: %w", interaction.ProcessID, err)
		}
	}

	output, err := process.wait(ctx, interaction.Wait, interaction.ProcessID)
	if err != nil {
		return machine.ProcessOutput{}, err
	}
	if output.Exited {
		m.removeProcess(interaction.ProcessID, process)
	}
	return output, nil
}

func (m *Local) startProcess(request machine.ProcessRequest, cols int, rows int, streamOutput bool) (*localProcess, error) {
	program := request.Argv[0]
	if program == "bash" {
		program = m.bash
	}
	cmd := exec.Command(program, request.Argv[1:]...)
	cmd.Dir = request.Dir
	if streamOutput {
		cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	}

	return m.startPreparedProcess(request, cmd, cols, rows, streamOutput, nil)
}

func (m *Local) startPreparedProcess(request machine.ProcessRequest, cmd *exec.Cmd, cols, rows int, streamOutput bool, cleanup func()) (*localProcess, error) {
	platform, err := newPlatformProcess()
	if err != nil {
		return nil, fmt.Errorf("machine-local: prepare process: %w", err)
	}
	platform.prepare(cmd, request.TTY)

	process := &localProcess{
		ownerID:    request.OwnerID,
		cleanup:    cleanup,
		cmd:        cmd,
		process:    platform,
		done:       make(chan struct{}),
		readerDone: make(chan struct{}),
	}
	if streamOutput {
		process.outputEvents = make(chan []byte, 32)
	}

	var output io.ReadCloser
	if request.TTY {
		terminal, ptyErr := xpty.NewPty(cols, rows)
		if ptyErr != nil {
			_ = platform.close()
			return nil, fmt.Errorf("machine-local: create terminal: %w", ptyErr)
		}
		process.pty = terminal
		output = terminal
		err = startTerminal(terminal, cmd, platform)
	} else {
		reader, writer, pipeErr := os.Pipe()
		if pipeErr != nil {
			_ = platform.close()
			return nil, fmt.Errorf("machine-local: create output pipe: %w", pipeErr)
		}
		output = reader
		cmd.Stdout = writer
		cmd.Stderr = writer
		err = cmd.Start()
		_ = writer.Close()
	}
	if err != nil {
		_ = output.Close()
		_ = platform.close()
		return nil, fmt.Errorf("machine-local: start %q: %w", request.Argv[0], err)
	}

	err = platform.started(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = output.Close()
		_ = xpty.WaitProcess(context.Background(), cmd)
		_ = platform.close()
		return nil, fmt.Errorf("machine-local: manage process tree: %w", err)
	}

	go process.readOutput(output, !request.TTY)
	go process.waitForExit()
	return process, nil
}

func (p *localProcess) readOutput(reader io.ReadCloser, closeWhenDone bool) {
	if closeWhenDone {
		defer reader.Close()
	}
	defer close(p.readerDone)
	if p.outputEvents != nil {
		defer close(p.outputEvents)
	}
	buffer := make([]byte, 32*1024)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			if p.outputEvents != nil {
				chunk := append([]byte(nil), buffer[:count]...)
				p.outputEvents <- chunk
			} else {
				p.mu.Lock()
				p.output.append(buffer[:count])
				p.mu.Unlock()
			}
		}
		if err != nil {
			return
		}
	}
}

func (p *localProcess) Output() <-chan []byte { return p.outputEvents }

func (p *localProcess) Write(data []byte) error {
	p.interactionMu.Lock()
	defer p.interactionMu.Unlock()
	if p.hasExited() {
		return fmt.Errorf("machine-local: terminal process has exited")
	}
	_, err := p.pty.Write(data)
	if err != nil {
		return fmt.Errorf("machine-local: write terminal: %w", err)
	}
	return nil
}

func (p *localProcess) CloseInput() error {
	// PTY 没有独立的写端；EOT 是交互式终端中的标准 EOF 输入。
	return p.Write([]byte{4})
}

func (p *localProcess) Resize(rows int, cols int) error {
	if rows <= 0 || cols <= 0 {
		return fmt.Errorf("machine-local: positive terminal size required")
	}
	p.interactionMu.Lock()
	defer p.interactionMu.Unlock()
	if p.hasExited() {
		return fmt.Errorf("machine-local: terminal process has exited")
	}
	err := p.pty.Resize(cols, rows)
	if err != nil {
		return fmt.Errorf("machine-local: resize terminal: %w", err)
	}
	return nil
}

func (p *localProcess) Terminate() error { return p.terminate() }

func (p *localProcess) Wait(ctx context.Context) (int, error) {
	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	case <-p.done:
	}
	p.mu.Lock()
	exitCode := p.exitCode
	p.mu.Unlock()
	return exitCode, nil
}

func (p *localProcess) waitForExit() {
	waitErr := xpty.WaitProcess(context.Background(), p.cmd)
	_ = p.process.close()
	if p.pty != nil {
		_ = p.pty.Close()
	}
	<-p.readerDone
	if p.cleanup != nil {
		p.cleanup()
	}

	exitCode := -1
	if p.cmd.ProcessState != nil {
		exitCode = p.cmd.ProcessState.ExitCode()
	} else if waitErr == nil {
		exitCode = 0
	}

	p.mu.Lock()
	p.exited = true
	p.exitCode = exitCode
	p.finishedAt = time.Now()
	p.mu.Unlock()
	close(p.done)
}

func (p *localProcess) wait(ctx context.Context, duration time.Duration, processID int64) (machine.ProcessOutput, error) {
	if !p.hasExited() {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return machine.ProcessOutput{}, ctx.Err()
		case <-p.done:
		case <-timer.C:
		}
	}

	p.mu.Lock()
	output, omitted := p.output.take()
	result := machine.ProcessOutput{
		ProcessID:    processID,
		Output:       output,
		Exited:       p.exited,
		ExitCode:     p.exitCode,
		OmittedBytes: omitted,
	}
	p.mu.Unlock()
	return result, nil
}

func (p *localProcess) hasExited() bool {
	p.mu.Lock()
	exited := p.exited
	p.mu.Unlock()
	return exited
}

func (p *localProcess) terminate() error {
	p.terminateMu.Lock()
	defer p.terminateMu.Unlock()
	if p.hasExited() || p.terminationSent {
		return nil
	}

	treeErr := p.process.terminate(p.cmd)
	if treeErr == nil {
		p.terminationSent = true
		return nil
	}

	rootErr := p.cmd.Process.Kill()
	if rootErr == nil || errors.Is(rootErr, os.ErrProcessDone) {
		p.terminationSent = true
		return nil
	}
	return errors.Join(
		fmt.Errorf("machine-local: terminate process tree: %w", treeErr),
		fmt.Errorf("machine-local: terminate root process: %w", rootErr),
	)
}

func (p *localProcess) waitUntil(deadline time.Time) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return fmt.Errorf("machine-local: timed out waiting for process to stop")
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-p.done:
		return nil
	case <-timer.C:
		return fmt.Errorf("machine-local: timed out waiting for process to stop")
	}
}

func (m *Local) removeProcess(processID int64, process *localProcess) {
	m.mu.Lock()
	if m.processes[processID] == process {
		delete(m.processes, processID)
	}
	m.mu.Unlock()
}

func (m *Local) pruneExitedProcessLocked() {
	var oldestID int64
	var oldestTime time.Time
	for processID, process := range m.processes {
		process.mu.Lock()
		exited := process.exited
		finishedAt := process.finishedAt
		process.mu.Unlock()
		if exited && (oldestID == 0 || finishedAt.Before(oldestTime)) {
			oldestID = processID
			oldestTime = finishedAt
		}
	}
	if oldestID != 0 {
		delete(m.processes, oldestID)
	}
}

func (m *Local) newProcessIDLocked() (int64, error) {
	var bytes [8]byte
	for range 32 {
		_, err := rand.Read(bytes[:])
		if err != nil {
			return 0, fmt.Errorf("machine-local: generate process ID: %w", err)
		}
		processID := int64(binary.LittleEndian.Uint64(bytes[:]) & uint64(maxSafeProcessID))
		if processID == 0 || m.processes[processID] != nil {
			continue
		}
		return processID, nil
	}
	return 0, errors.New("machine-local: unable to allocate process ID")
}

func processNotFound(processID int64) error {
	return fmt.Errorf("machine-local: process %d not found", processID)
}

type headTailBuffer struct {
	head      []byte
	tail      []byte
	omitted   int64
	truncated bool
}

func (b *headTailBuffer) append(data []byte) {
	const half = maxProcessOutput / 2
	if !b.truncated && len(b.head)+len(data) <= maxProcessOutput {
		b.head = append(b.head, data...)
		return
	}
	if !b.truncated {
		combined := make([]byte, 0, len(b.head)+len(data))
		combined = append(combined, b.head...)
		combined = append(combined, data...)
		b.head = append(b.head[:0], combined[:half]...)
		b.tail = append(b.tail[:0], combined[len(combined)-half:]...)
		b.omitted = int64(len(combined) - maxProcessOutput)
		b.truncated = true
		return
	}

	if len(b.tail)+len(data) <= half {
		b.tail = append(b.tail, data...)
		return
	}
	combined := make([]byte, 0, len(b.tail)+len(data))
	combined = append(combined, b.tail...)
	combined = append(combined, data...)
	b.omitted += int64(len(combined) - half)
	b.tail = append(b.tail[:0], combined[len(combined)-half:]...)
}

func (b *headTailBuffer) take() ([]byte, int64) {
	output := make([]byte, 0, len(b.head)+len(b.tail))
	output = append(output, b.head...)
	output = append(output, b.tail...)
	omitted := b.omitted
	b.head = b.head[:0]
	b.tail = b.tail[:0]
	b.omitted = 0
	b.truncated = false
	return output, omitted
}
