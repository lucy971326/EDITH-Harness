//go:build !linux && !darwin

package machinelocal

import (
	"fmt"
	"harness/internal/permissions"
	"runtime"
)

func prepareSandbox(_ permissions.Policy, _ string, _ []string) (*agentLaunch, error) {
	return nil, fmt.Errorf("machine: restricted Agent execution is not implemented on %s; select Full Access explicitly to run without a sandbox", runtime.GOOS)
}
