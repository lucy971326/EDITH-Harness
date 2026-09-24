package appserver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"harness/internal/appserver/internal/clientconn"
	"harness/internal/machine"
)

const (
	commandExecMethod       = "command/exec"
	commandExecWriteMethod  = "command/exec/write"
	commandExecResizeMethod = "command/exec/resize"
	commandExecStopMethod   = "command/exec/terminate"
	commandExecOutputMethod = "command/exec/outputDelta"
)

// 数据。PTY 的字符行列数。
type CommandExecTerminalSize struct {
	Rows int `json:"rows" jsonschema:"minimum=1,maximum=1000"`
	Cols int `json:"cols" jsonschema:"minimum=1,maximum=1000"`
}

// 数据。启动当前工作区中的交互式 Bash。
type CommandExecParams struct {
	ProcessID string                  `json:"processId" jsonschema:"minLength=1,maxLength=128"`
	CWD       string                  `json:"cwd" jsonschema:"minLength=1"`
	Size      CommandExecTerminalSize `json:"size"`
}

// 数据。交互式 Bash 退出后的结果。
type CommandExecResult struct {
	ExitCode int `json:"exitCode"`
}

// 数据。向正在运行的终端写入字节或发送 EOF。
type CommandExecWriteParams struct {
	ProcessID   string  `json:"processId" jsonschema:"minLength=1,maxLength=128"`
	DeltaBase64 *string `json:"deltaBase64,omitempty"`
	CloseStdin  bool    `json:"closeStdin,omitempty"`
}

// 数据。调整正在运行的终端尺寸。
type CommandExecResizeParams struct {
	ProcessID string                  `json:"processId" jsonschema:"minLength=1,maxLength=128"`
	Size      CommandExecTerminalSize `json:"size"`
}

// 数据。终止正在运行的终端。
type CommandExecTerminateParams struct {
	ProcessID string `json:"processId" jsonschema:"minLength=1,maxLength=128"`
}

// 数据。PTY 合并输出的一段字节。
type CommandExecOutputDelta struct {
	ProcessID   string `json:"processId"`
	Stream      string `json:"stream"`
	DeltaBase64 string `json:"deltaBase64"`
	CapReached  bool   `json:"capReached"`
}

type commandProcessKey struct {
	connectionID string
	processID    string
}

type commandControl struct {
	write      []byte
	closeInput bool
	resize     *CommandExecTerminalSize
	terminate  bool
	result     chan error
}

type commandSession struct {
	process  machine.TerminalProcess
	controls chan commandControl
	done     chan struct{}

	mu       sync.Mutex
	exitCode int
	err      error
}

type commandExecManager struct {
	terminals machine.TerminalSystem

	mu       sync.Mutex
	sessions map[commandProcessKey]*commandSession
}

// BindCommandExec 显式接入 machine 的低层 PTY 能力。
func (s *Server) BindCommandExec(terminals machine.TerminalSystem) error {
	if terminals == nil || s.commandExec != nil {
		return fmt.Errorf("appserver: nil or already bound command exec")
	}
	s.commandExec = &commandExecManager{
		terminals: terminals,
		sessions:  make(map[commandProcessKey]*commandSession),
	}
	if err := Register(s, commandExecMethod, s.handleCommandExec); err != nil {
		return err
	}
	if err := Register(s, commandExecWriteMethod, s.handleCommandExecWrite); err != nil {
		return err
	}
	if err := Register(s, commandExecResizeMethod, s.handleCommandExecResize); err != nil {
		return err
	}
	return Register(s, commandExecStopMethod, s.handleCommandExecTerminate)
}

func (s *Server) handleCommandExec(ctx context.Context, input CommandExecParams) (CommandExecResult, error) {
	request, err := clientconn.FromContext(ctx)
	if err != nil {
		return CommandExecResult{}, err
	}
	if !filepath.IsAbs(input.CWD) {
		return CommandExecResult{}, &Error{Code: CodeInvalidParams, Message: "absolute cwd required"}
	}

	key := commandProcessKey{connectionID: request.ConnectionID(), processID: input.ProcessID}
	session, err := s.commandExec.start(ctx, request, key, input)
	if err != nil {
		return CommandExecResult{}, err
	}

	select {
	case <-ctx.Done():
		_ = session.process.Terminate()
		<-session.done
		return CommandExecResult{}, ctx.Err()
	case <-session.done:
	}

	session.mu.Lock()
	exitCode := session.exitCode
	runErr := session.err
	session.mu.Unlock()
	if runErr != nil {
		return CommandExecResult{}, runErr
	}
	if err := request.FlushNotifications(ctx); err != nil {
		return CommandExecResult{}, err
	}
	return CommandExecResult{ExitCode: exitCode}, nil
}

func (s *Server) handleCommandExecWrite(ctx context.Context, input CommandExecWriteParams) (struct{}, error) {
	if input.DeltaBase64 == nil && !input.CloseStdin {
		return struct{}{}, &Error{Code: CodeInvalidParams, Message: "deltaBase64 or closeStdin required"}
	}
	var data []byte
	if input.DeltaBase64 != nil {
		var err error
		data, err = base64.StdEncoding.DecodeString(*input.DeltaBase64)
		if err != nil {
			return struct{}{}, &Error{Code: CodeInvalidParams, Message: "terminal input is not valid base64", Cause: err}
		}
	}
	err := s.commandExec.control(ctx, input.ProcessID, commandControl{write: data, closeInput: input.CloseStdin})
	return struct{}{}, err
}

func (s *Server) handleCommandExecResize(ctx context.Context, input CommandExecResizeParams) (struct{}, error) {
	err := s.commandExec.control(ctx, input.ProcessID, commandControl{resize: &input.Size})
	return struct{}{}, err
}

func (s *Server) handleCommandExecTerminate(ctx context.Context, input CommandExecTerminateParams) (struct{}, error) {
	err := s.commandExec.control(ctx, input.ProcessID, commandControl{terminate: true})
	return struct{}{}, err
}

func (m *commandExecManager) start(ctx context.Context, request *clientconn.RequestContext, key commandProcessKey, input CommandExecParams) (*commandSession, error) {
	m.mu.Lock()
	if m.sessions[key] != nil {
		m.mu.Unlock()
		return nil, &Error{Code: CodeConflict, Message: "processId is already running"}
	}
	process, err := m.terminals.StartTerminal(machine.TerminalRequest{
		Dir:  input.CWD,
		Argv: []string{"bash", "--login", "-i"},
		Rows: input.Size.Rows,
		Cols: input.Size.Cols,
	})
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	session := &commandSession{
		process:  process,
		controls: make(chan commandControl),
		done:     make(chan struct{}),
	}
	m.sessions[key] = session
	m.mu.Unlock()

	go m.run(ctx, request, key, session)
	return session, nil
}

func (m *commandExecManager) run(ctx context.Context, request *clientconn.RequestContext, key commandProcessKey, session *commandSession) {
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		for output := range session.process.Output() {
			if !request.Notify(commandExecOutputMethod, CommandExecOutputDelta{
				ProcessID:   key.processID,
				Stream:      "stdout",
				DeltaBase64: base64.StdEncoding.EncodeToString(output),
			}) {
				_ = session.process.Terminate()
			}
		}
	}()

	waitResult := make(chan struct {
		exitCode int
		err      error
	}, 1)
	go func() {
		exitCode, err := session.process.Wait(context.Background())
		waitResult <- struct {
			exitCode int
			err      error
		}{exitCode: exitCode, err: err}
	}()

	requestDone := request.Done()
	runDone := ctx.Done()
	var result struct {
		exitCode int
		err      error
	}
	for {
		select {
		case <-runDone:
			_ = session.process.Terminate()
			runDone = nil
		case <-requestDone:
			_ = session.process.Terminate()
			requestDone = nil
		case control := <-session.controls:
			control.result <- applyCommandControl(session.process, control)
		case result = <-waitResult:
			<-outputDone
			session.mu.Lock()
			session.exitCode = result.exitCode
			session.err = result.err
			session.mu.Unlock()
			m.mu.Lock()
			if m.sessions[key] == session {
				delete(m.sessions, key)
			}
			m.mu.Unlock()
			close(session.done)
			return
		}
	}
}

func applyCommandControl(process machine.TerminalProcess, control commandControl) error {
	var result error
	if len(control.write) > 0 {
		result = process.Write(control.write)
	}
	if control.closeInput {
		result = errors.Join(result, process.CloseInput())
	}
	if control.resize != nil {
		result = errors.Join(result, process.Resize(control.resize.Rows, control.resize.Cols))
	}
	if control.terminate {
		result = errors.Join(result, process.Terminate())
	}
	return result
}

func (m *commandExecManager) control(ctx context.Context, processID string, control commandControl) error {
	request, err := clientconn.FromContext(ctx)
	if err != nil {
		return err
	}
	key := commandProcessKey{connectionID: request.ConnectionID(), processID: processID}
	m.mu.Lock()
	session := m.sessions[key]
	m.mu.Unlock()
	if session == nil {
		return &Error{Code: CodeNotFound, Message: "terminal process not found"}
	}

	control.result = make(chan error, 1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-session.done:
		return &Error{Code: CodeNotFound, Message: "terminal process is no longer running"}
	case session.controls <- control:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-session.done:
		return &Error{Code: CodeNotFound, Message: "terminal process is no longer running"}
	case err := <-control.result:
		return err
	}
}
