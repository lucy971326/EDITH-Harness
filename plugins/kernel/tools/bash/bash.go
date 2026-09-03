package bash

import (
	"context"
	"strings"

	"harness/kernel/machine"
	"harness/kernel/tools"
)

// 数据。bash 工具的模型参数。
type Args struct {
	Command string `json:"command" jsonschema:"minLength=1,description=Bash command to execute in the workspace."`
}

func newTool(m machine.Machine) tools.Tool {
	return tools.New("bash", "Run a bash command in the workspace. Returns stdout and stderr.", func(ctx context.Context, call tools.Call, args Args) (tools.Result, error) {
		stdout, stderr, err := m.Run(ctx, call.Workspace, []string{"bash", "-c", args.Command})
		if ctxErr := ctx.Err(); ctxErr != nil {
			return tools.Result{}, ctxErr
		}

		output := formatOutput(stdout, stderr)
		if err != nil {
			return tools.Result{Content: tools.TruncateTail(output + "\n\n" + err.Error()), IsError: true}, nil
		}
		return tools.Result{Content: tools.TruncateTail(output)}, nil
	})
}

func formatOutput(stdout []byte, stderr []byte) string {
	var parts []string
	if len(stdout) > 0 {
		parts = append(parts, "stdout:\n"+string(stdout))
	}
	if len(stderr) > 0 {
		parts = append(parts, "stderr:\n"+string(stderr))
	}
	if len(parts) == 0 {
		return "(no output)"
	}
	return strings.Join(parts, "\n")
}
