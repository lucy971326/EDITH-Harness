package machinelocal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"harness/internal/permissions"
)

func prepareSandbox(policy permissions.Policy, dir string, argv []string) (*agentLaunch, error) {
	const executable = "/usr/bin/sandbox-exec"
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("machine: system Seatbelt launcher is required at %s", executable)
	}

	// 只解析系统顶层别名；工作区内部的符号链接不能变成新的授权目标。
	roots := make([]string, 0, len(policy.WriteRoots))
	for _, root := range policy.WriteRoots {
		parts := strings.Split(strings.TrimPrefix(root, "/"), "/")
		top := "/" + parts[0]
		resolvedTop, resolveErr := filepath.EvalSymlinks(top)
		if resolveErr != nil {
			return nil, fmt.Errorf("machine: writable root %s: %w", root, resolveErr)
		}
		normalized := filepath.Join(append([]string{resolvedTop}, parts[1:]...)...)
		resolved, resolveErr := filepath.EvalSymlinks(normalized)
		if resolveErr != nil || resolved != normalized {
			return nil, fmt.Errorf("machine: writable root must exist and not cross a nested symlink: %s", root)
		}
		info, statErr := os.Stat(normalized)
		if statErr != nil || !info.IsDir() || normalized == "/" || normalized == "/dev" || normalized == "/System" {
			return nil, fmt.Errorf("machine: unsupported writable directory %s", root)
		}
		roots = append(roots, normalized)
	}
	// 在实际路径上重新计算保护，避免 /var 与 /private/var 两种拼写产生不同权限。
	normalizedPolicy := policy
	normalizedPolicy.WriteRoots = roots
	var protected []string
	for _, root := range roots {
		for _, name := range []string{".git", ".agents", ".harness"} {
			path := filepath.Join(root, name)
			requirement, err := permissions.Evaluate(normalizedPolicy, permissions.ExtraPermissions{WriteRoots: []string{path}})
			if err != nil {
				return nil, err
			}
			if requirement == permissions.Allow {
				continue
			}
			for _, child := range roots {
				if strings.HasPrefix(child, path+"/") {
					return nil, fmt.Errorf("machine: nested grant inside protected path requires a separate sandbox policy: %s", child)
				}
			}
			info, err := os.Lstat(path)
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			if err == nil && info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("machine: protected path cannot be a symlink: %s", path)
			}
			protected = append(protected, path)
		}
	}

	// 路径作为 -D 参数传入，不插入 SBPL 源码，也不经过 Shell 再解释。
	profile := seatbeltBasePolicy
	var parameters []string
	for index, root := range roots {
		key := fmt.Sprintf("ROOT_%d", index)
		parameters = append(parameters, "-D"+key+"="+root)
		profile += fmt.Sprintf("\n(allow file-write* (subpath (param %q)))", key)
		// 固定授权边界及其祖先，防止 rename 将受保护内容搬出路径规则。
		for ancestor := root; ancestor != "/"; ancestor = filepath.Dir(ancestor) {
			key := fmt.Sprintf("ANCHOR_%d", len(parameters))
			parameters = append(parameters, "-D"+key+"="+ancestor)
			profile += fmt.Sprintf("\n(deny file-write-unlink (literal (param %q)))", key)
		}
	}
	for index, path := range protected {
		key := fmt.Sprintf("PROTECTED_%d", index)
		parameters = append(parameters, "-D"+key+"="+path)
		profile += fmt.Sprintf("\n(deny file-write* (literal (param %q)) (subpath (param %q)))", key, key)
	}
	if policy.Network {
		profile += seatbeltNetworkPolicy
	}
	args := append([]string{"-p", profile}, parameters...)
	args = append(args, "--")
	args = append(args, argv...)
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	return &agentLaunch{cmd: cmd}, nil
}

// 参考 Codex sandboxing 的 Seatbelt 基础规则；仅开放当前机器通道需要的能力。
// 子进程继承限制；普通文件只读，写入范围由上面的参数化规则添加。
const seatbeltBasePolicy = `(version 1)
(deny default)
(allow file-read*)
(allow process-exec)
(allow process-fork)
(allow signal (target same-sandbox))
(allow process-info* (target same-sandbox))
(allow sysctl-read)
(allow mach-lookup (global-name "com.apple.system.opendirectoryd.libinfo"))
(allow file-write-data (require-all (literal "/dev/null") (vnode-type CHARACTER-DEVICE)))
(allow pseudo-tty)
(allow file-read* file-write* file-ioctl (literal "/dev/ptmx"))
(allow file-read* file-write* (require-all (regex #"^/dev/ttys[0-9]+$") (extension "com.apple.sandbox.pty")))
(allow file-ioctl (regex #"^/dev/ttys[0-9]+$"))
(deny mach-lookup (xpc-service-name-prefix ""))
; 普通 file-write 限制不能覆盖这两种通过只读描述符修改文件的操作。
(deny system-fcntl (fcntl-command 80 110))
`

const seatbeltNetworkPolicy = `
(allow network-outbound)
(allow network-inbound)
(allow system-socket (require-all (socket-domain AF_SYSTEM) (socket-protocol 2)))
(allow mach-lookup
  (global-name "com.apple.bsd.dirhelper")
  (global-name "com.apple.system.opendirectoryd.membership")
  (global-name "com.apple.SecurityServer")
  (global-name "com.apple.networkd")
  (global-name "com.apple.ocspd")
  (global-name "com.apple.trustd.agent")
  (global-name "com.apple.SystemConfiguration.DNSConfiguration")
  (global-name "com.apple.SystemConfiguration.configd"))
`
