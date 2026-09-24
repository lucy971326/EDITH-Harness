package machinelocal

import (
	"fmt"
	"os"
	"os/exec"

	"harness/kernel/machine"
	"harness/kernel/permissions"
)

// 启动方案保存命令及平台资源；Linux 的挂载源、过滤器和临时占位在退出后释放。
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

func (m *Local) prepareAgentLaunch(policy permissions.Policy, request machine.ProcessRequest) (*agentLaunch, error) {
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
