package machinelocal

import (
	"fmt"
	"os"
	"os/exec"

	"harness/kernel/machine"
	"harness/kernel/permissions"
)

// 启动方案拥有挂载源、过滤器和临时占位，进程退出后统一释放。
type agentLaunch struct {
	cmd     *exec.Cmd
	files   []*os.File
	release func()
}

func (l *agentLaunch) close() {
	for _, file := range l.files {
		_ = file.Close()
	}
	if l.release != nil {
		l.release()
	}
}

func (m *local) prepareAgentLaunch(policy permissions.Policy, request machine.ProcessRequest) (*agentLaunch, error) {
	if len(request.Argv) == 0 {
		return nil, fmt.Errorf("machine: empty command")
	}
	_, err := permissions.Evaluate(policy, permissions.ExtraPermissions{})
	if err != nil {
		return nil, err
	}
	argv := append([]string(nil), request.Argv...)
	if argv[0] == "bash" {
		argv[0] = m.bash
	}
	if policy.Unrestricted {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = request.Dir
		return &agentLaunch{cmd: cmd}, nil
	}
	return prepareSandbox(policy, request.Dir, argv)
}
