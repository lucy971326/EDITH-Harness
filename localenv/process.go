package localenv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"harness/workspace/process"
)

// localSpawner 在同一个本地根目录下启动进程，并跟踪它们用于统一收摊。
type localSpawner struct {
	mu     sync.Mutex                // 保护跟踪表和关闭状态
	root   string                    // 新进程的默认工作目录
	active map[*localHandle]struct{} // 还没退出的进程
	closed bool                      // 关闭后拒绝新进程
}

// localHandle 组合一个本地进程、三条管道和可重复等待的结果。
type localHandle struct {
	owner  *localSpawner   // 所属跟踪表
	cmd    *exec.Cmd       // 真实本地进程
	ctx    context.Context // 进程生命期
	stdin  io.WriteCloser  // 持续输入
	stdout io.ReadCloser   // 标准输出
	stderr io.ReadCloser   // 错误输出
	done   chan struct{}   // exec.Wait 完成后关闭

	mu        sync.Mutex // 保护结束结果
	cancelErr error      // ctx 取消或超时原因
	waitErr   error      // exec.Wait 的原始错误
	exit      process.Exit

	closeOnce sync.Once // 整个进程组只杀一次
	closeErr  error     // 第一次关闭的结果
}

// newSpawner 返回一个以 root 为默认工作目录的本地进程能力。
func newSpawner(root string) (process.Spawner, error) {
	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析进程根目录失败：%w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("打开进程根目录失败：%w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("进程根路径 %s 不是目录", absolute)
	}
	return &localSpawner{
		root:   absolute,
		active: make(map[*localHandle]struct{}),
	}, nil
}

// Spawn 启动一个新进程；ctx 取消会收掉它的整个进程组。
func (s *localSpawner) Spawn(ctx context.Context, command process.Command) (process.Handle, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err
	}
	if command.Program == "" {
		return nil, errors.New("进程程序名不能为空")
	}
	dir, err := s.resolveDir(command.Dir)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(command.Program, command.Args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), command.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("打开进程 stdin 失败：%w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("打开进程 stdout 失败：%w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("打开进程 stderr 失败：%w", err)
	}

	handle := &localHandle{
		owner:  s,
		cmd:    cmd,
		ctx:    ctx,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan struct{}),
		exit:   process.Exit{Code: -1},
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, errors.New("Spawner 已关闭")
	}
	err = cmd.Start()
	if err != nil {
		s.mu.Unlock()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("启动进程 %s 失败：%w", command.Program, err)
	}
	s.active[handle] = struct{}{}
	s.mu.Unlock()

	go handle.watch()
	go handle.stopWhenCancelled()
	return handle, nil
}

// Close 拒绝新进程，并杀掉、等完当前跟踪的所有进程树。
func (s *localSpawner) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	active := make([]*localHandle, 0, len(s.active))
	for handle := range s.active {
		active = append(active, handle)
	}
	s.mu.Unlock()

	var closeErrs []error
	for _, handle := range active {
		err := handle.Close()
		if err != nil {
			closeErrs = append(closeErrs, err)
		}
	}
	return errors.Join(closeErrs...)
}

// resolveDir 把进程工作目录放到根目录下。
func (s *localSpawner) resolveDir(name string) (string, error) {
	if name == "" {
		return s.root, nil
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("进程工作目录 %s 必须是相对路径", name)
	}
	clean := filepath.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("进程工作目录 %s 超出根目录", name)
	}
	path := filepath.Join(s.root, clean)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("打开进程工作目录 %s 失败：%w", name, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("进程工作路径 %s 不是目录", name)
	}
	return path, nil
}

// remove 在进程结束后把它从跟踪表删掉。
func (s *localSpawner) remove(handle *localHandle) {
	s.mu.Lock()
	delete(s.active, handle)
	s.mu.Unlock()
}

func (p *localHandle) Stdin() io.WriteCloser { return p.stdin }
func (p *localHandle) Stdout() io.Reader     { return p.stdout }
func (p *localHandle) Stderr() io.Reader     { return p.stderr }

// Signal 向进程组发一个信号，让它的子进程一起收到。
func (p *localHandle) Signal(signal os.Signal) error {
	select {
	case <-p.done:
		return os.ErrProcessDone
	default:
	}
	return signalGroup(p.cmd.Process.Pid, signal)
}

// Wait 可重复等待进程结束，返回退出码和真实的结束原因。
func (p *localHandle) Wait() (process.Exit, error) {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancelErr != nil {
		return p.exit, p.cancelErr
	}
	return p.exit, p.waitErr
}

// Close 杀掉整个进程组并等它们退出；可重复调用。
func (p *localHandle) Close() error {
	p.closeOnce.Do(func() {
		_ = p.stdin.Close()
		p.closeErr = signalGroup(p.cmd.Process.Pid, os.Kill)
		if p.closeErr == nil {
			<-p.done
		}
	})
	return p.closeErr
}

// watch 只调一次 exec.Wait，再把结果变成可重复读取的状态。
func (p *localHandle) watch() {
	err := p.cmd.Wait()
	exit := process.Exit{Code: -1}
	if p.cmd.ProcessState != nil {
		exit.Code = p.cmd.ProcessState.ExitCode()
	}

	p.mu.Lock()
	p.waitErr = err
	p.exit = exit
	p.mu.Unlock()
	close(p.done)
	p.owner.remove(p)
}

// stopWhenCancelled 在 ctx 取消后杀掉进程组，让 Wait 返回 ctx 的原因。
func (p *localHandle) stopWhenCancelled() {
	select {
	case <-p.done:
		return
	case <-p.ctx.Done():
	}

	p.mu.Lock()
	p.cancelErr = p.ctx.Err()
	p.mu.Unlock()
	_ = p.Close()
}

// signalGroup 向本地 Unix 进程组发信号。
func signalGroup(pid int, signal os.Signal) error {
	systemSignal, ok := signal.(syscall.Signal)
	if !ok {
		return fmt.Errorf("不支持的进程信号 %v", signal)
	}
	err := syscall.Kill(-pid, systemSignal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
