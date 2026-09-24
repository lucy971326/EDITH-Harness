package machinelocal

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"

	"harness/kernel/permissions"

	"golang.org/x/sys/unix"
)

// 占位只属于当前宿主；并发沙箱共用，最后一个退出后按身份清理。
var sandboxMounts = struct {
	sync.Mutex
	entries map[string]*sandboxPlaceholder
}{entries: make(map[string]*sandboxPlaceholder)}

type sandboxPlaceholder struct {
	info  os.FileInfo
	users int
}

func prepareSandbox(policy permissions.Policy, dir string, argv []string) (_ *agentLaunch, resultErr error) {
	// 固定系统位置，防止工作区或用户 PATH 中的同名程序冒充 bwrap。
	bwrap := "/usr/bin/bwrap"
	info, err := os.Stat(bwrap)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("machine: system bubblewrap is required at %s", bwrap)
	}
	launch := &agentLaunch{}
	defer func() {
		if resultErr != nil {
			launch.close()
		}
	}()
	args := []string{"--die-with-parent", "--new-session", "--unshare-user", "--unshare-pid", "--unshare-ipc", "--cap-drop", "ALL", "--ro-bind", "/", "/"}
	if !policy.Network {
		args = append(args, "--unshare-net")
	}

	// 固定挂载源描述符；可写根必须是实际目录，保护路径不允许穿越符号链接。
	roots := append([]string(nil), policy.WriteRoots...)
	sort.Strings(roots)
	seen := make(map[string]bool)
	var held []string
	launch.release = func() { releaseSandboxPlaceholders(held) }
	for _, root := range roots {
		resolved, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr != nil {
			return nil, fmt.Errorf("machine: writable root %s: %w", root, resolveErr)
		}
		if resolved != root {
			return nil, fmt.Errorf("machine: writable root must not cross a symlink: %s", root)
		}
		if root == "/" || root == "/proc" || root == "/sys" || root == "/dev" {
			return nil, fmt.Errorf("machine: unsupported writable root %s", root)
		}
		if seen[root] {
			continue
		}
		seen[root] = true
		info, statErr := os.Stat(root)
		if statErr != nil || !info.IsDir() {
			return nil, fmt.Errorf("machine: writable root must be an existing directory: %s", root)
		}
		err = appendSandboxBind(launch, &args, root, false)
		if err != nil {
			return nil, err
		}
		for _, name := range []string{".git", ".agents", ".harness"} {
			path := filepath.Join(root, name)
			requirement, evalErr := permissions.Evaluate(policy, permissions.ExtraPermissions{WriteRoots: []string{path}})
			if evalErr != nil {
				return nil, evalErr
			}
			if requirement == permissions.Allow {
				continue
			}
			err = holdSandboxPlaceholder(path)
			if err != nil {
				return nil, err
			}
			held = append(held, path)
			err = appendSandboxBind(launch, &args, path, true)
			if err != nil {
				return nil, err
			}
		}
	}
	// 后续子根不能通过覆盖挂载解除父根的保护。
	for _, root := range roots {
		for _, name := range []string{".git", ".agents", ".harness"} {
			path := filepath.Join(root, name)
			requirement, _ := permissions.Evaluate(policy, permissions.ExtraPermissions{WriteRoots: []string{path}})
			if requirement == permissions.Allow {
				continue
			}
			for _, child := range roots {
				if child != root && pathWithin(path, child) {
					return nil, fmt.Errorf("machine: nested grant inside protected path requires a separate sandbox policy: %s", child)
				}
			}
		}
	}
	filter, err := sandboxFilter(policy.Network)
	if err != nil {
		return nil, err
	}
	fd, err := unix.MemfdCreate("harness-seccomp", unix.MFD_CLOEXEC)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "harness-seccomp")
	launch.files = append(launch.files, file)
	_, err = file.Write(filter)
	if err != nil {
		return nil, err
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		return nil, err
	}
	args = append(args, "--proc", "/proc", "--dev", "/dev", "--seccomp", strconv.Itoa(2+len(launch.files)))
	if dir != "" {
		args = append(args, "--chdir", dir)
	}
	args = append(args, "--")
	args = append(args, argv...)
	launch.cmd = exec.Command(bwrap, args...)
	launch.cmd.Dir = dir
	launch.cmd.ExtraFiles = launch.files
	return launch, nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && (len(rel) < 3 || rel[:3] != "../")
}

func appendSandboxBind(launch *agentLaunch, args *[]string, path string, readonly bool) error {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("machine: open mount %s: %w", path, err)
	}
	file := os.NewFile(uintptr(fd), path)
	launch.files = append(launch.files, file)
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("machine: protected mount cannot be a symlink: %s", path)
	}
	flag := "--bind-fd"
	if readonly {
		flag = "--ro-bind-fd"
	}
	*args = append(*args, flag, strconv.Itoa(2+len(launch.files)), path)
	return nil
}

func holdSandboxPlaceholder(path string) error {
	sandboxMounts.Lock()
	defer sandboxMounts.Unlock()
	if entry := sandboxMounts.entries[path]; entry != nil {
		entry.users++
		return nil
	}
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("machine: protected path cannot be a symlink: %s", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	err = os.Mkdir(path, 0o755)
	if os.IsExist(err) {
		return fmt.Errorf("machine: protected path changed during sandbox setup: %s", path)
	}
	if err != nil {
		return err
	}
	info, err = os.Lstat(path)
	if err != nil {
		return err
	}
	sandboxMounts.entries[path] = &sandboxPlaceholder{info: info, users: 1}
	return nil
}

func releaseSandboxPlaceholders(paths []string) {
	sandboxMounts.Lock()
	defer sandboxMounts.Unlock()
	for _, path := range paths {
		entry := sandboxMounts.entries[path]
		if entry == nil {
			continue
		}
		entry.users--
		if entry.users != 0 {
			continue
		}
		delete(sandboxMounts.entries, path)
		info, err := os.Lstat(path)
		if err == nil && os.SameFile(info, entry.info) {
			_ = os.Remove(path)
		} // Remove 不会删除非空目录。
	}
}

// 只允许本机 ABI；过滤器由 bwrap 在 exec 前安装，不影响宿主 Go 线程。
func sandboxFilter(network bool) ([]byte, error) {
	var arch uint32
	switch runtime.GOARCH {
	case "amd64":
		arch = unix.AUDIT_ARCH_X86_64
	case "arm64":
		arch = unix.AUDIT_ARCH_AARCH64
	default:
		return nil, fmt.Errorf("machine: seccomp unsupported on %s", runtime.GOARCH)
	}
	instructions := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 4},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 1, K: arch},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_KILL_PROCESS},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
	}
	if runtime.GOARCH == "amd64" {
		instructions = append(instructions, unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, Jf: 1, K: 0x40000000}, unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_KILL_PROCESS})
	}
	denied := []uint32{unix.SYS_PTRACE, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER, unix.SYS_MOUNT, unix.SYS_UMOUNT2, unix.SYS_PIVOT_ROOT, unix.SYS_SETNS, unix.SYS_UNSHARE, unix.SYS_FSOPEN, unix.SYS_FSCONFIG, unix.SYS_FSMOUNT, unix.SYS_FSPICK, unix.SYS_OPEN_TREE, unix.SYS_MOVE_MOUNT, unix.SYS_MOUNT_SETATTR}
	if !network {
		denied = append(denied, unix.SYS_CONNECT, unix.SYS_BIND, unix.SYS_LISTEN, unix.SYS_ACCEPT, unix.SYS_ACCEPT4, unix.SYS_SENDTO, unix.SYS_SENDMSG, unix.SYS_SENDMMSG)
	}
	for _, number := range denied {
		instructions = append(instructions, unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: 1, K: number}, unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)})
	}
	// socketpair 只保留面向连接的流，避免 datagram 带目标地址绕过宿主 socket 隔离。
	instructions = append(instructions,
		unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: 5, K: unix.SYS_SOCKETPAIR},
		unix.SockFilter{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 24},
		unix.SockFilter{Code: unix.BPF_ALU | unix.BPF_AND | unix.BPF_K, K: 0xf},
		unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 1, K: unix.SOCK_STREAM},
		unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)},
		unix.SockFilter{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
	)
	// socketpair(AF_UNIX) 保留给父子进程通信；联网模式也不允许连接宿主 Unix socket。
	for _, number := range []uint32{unix.SYS_SOCKET, unix.SYS_SOCKETPAIR} {
		allowed := []uint32{unix.AF_UNIX}
		if network && number == unix.SYS_SOCKET {
			allowed = []uint32{unix.AF_INET, unix.AF_INET6}
		}
		instructions = append(instructions, unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: uint8(len(allowed) + 3), K: number}, unix.SockFilter{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 16})
		for i, family := range allowed {
			instructions = append(instructions, unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: uint8(len(allowed) - i), K: family})
		}
		instructions = append(instructions, unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)}, unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW})
	}
	instructions = append(instructions, unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW})
	var buffer bytes.Buffer
	err := binary.Write(&buffer, binary.LittleEndian, instructions)
	return buffer.Bytes(), err
}
