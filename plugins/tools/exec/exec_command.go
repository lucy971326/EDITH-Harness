package exec

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"harness/kernel/machine"
	"harness/kernel/tools"
)

const (
	defaultExecYieldMS = int64(10_000)
	minExecYieldMS     = int64(250)
	minWindowsYieldMS  = int64(10_000)
	maxExecYieldMS     = int64(30_000)
)

// 数据。exec_command 的模型参数。
type ExecCommandArgs struct {
	Cmd             string `json:"cmd" jsonschema:"minLength=1,description=Shell command to execute."`
	Workdir         string `json:"workdir,omitempty" jsonschema:"description=Working directory for the command. Defaults to the current workspace."`
	TTY             bool   `json:"tty,omitempty" jsonschema:"description=True allocates a PTY; false or omitted uses plain pipes."`
	YieldTimeMS     *int64 `json:"yield_time_ms,omitempty" jsonschema:"description=Maximum time to wait before returning a process ID for a still-running command."`
	MaxOutputTokens *int   `json:"max_output_tokens,omitempty" jsonschema:"minimum=64,description=Output token budget. Defaults to 10000 tokens."`
}

func newExecCommandTool(processes machine.ProcessSystem, paths machine.Machine) tools.Tool {
	return tools.New(
		"exec_command",
		"Run a Bash command, returning output or a process ID for ongoing interaction.",
		func(ctx context.Context, call tools.Call, args ExecCommandArgs) (tools.Result, error) {
			maxTokens, err := outputTokenBudget(args.MaxOutputTokens)
			if err != nil {
				return tools.Result{}, err
			}

			workdir := call.Workspace
			if args.Workdir != "" {
				workdir = paths.ResolvePath(call.Workspace, args.Workdir)
			}
			shellFlag := "-lc"
			if args.TTY {
				shellFlag = "-lic"
			}
			yieldMS := defaultExecYieldMS
			if args.YieldTimeMS != nil {
				yieldMS = *args.YieldTimeMS
			}
			minimum := minExecYieldMS
			if runtime.GOOS == "windows" {
				minimum = minWindowsYieldMS
			}
			yieldMS = clamp(yieldMS, minimum, maxExecYieldMS)

			started := time.Now()
			output, err := processes.Exec(ctx, machine.ProcessRequest{
				OwnerID: call.SessionID,
				Dir:     workdir,
				Argv:    []string{"bash", shellFlag, args.Cmd},
				TTY:     args.TTY,
				Wait:    time.Duration(yieldMS) * time.Millisecond,
			})
			if err != nil {
				return tools.Result{}, fmt.Errorf("exec_command: %w", err)
			}
			return tools.Result{Content: renderProcessOutput(output, time.Since(started), maxTokens)}, nil
		},
	)
}
