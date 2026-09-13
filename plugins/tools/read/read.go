package read

import (
	"context"

	"harness/kernel/machine"
	"harness/kernel/tools"
)

// 数据。read 工具的模型参数。
type Args struct {
	Path string `json:"path" jsonschema:"minLength=1,description=Path to the file to read. Relative paths start at the workspace."`
}

func newTool(m machine.Machine) tools.Tool {
	return tools.New("read", "Read a text file. Output is limited to 2000 lines or 50 KiB.", func(ctx context.Context, call tools.Call, args Args) (tools.Result, error) {
		if err := ctx.Err(); err != nil {
			return tools.Result{}, err
		}
		path := m.ResolvePath(call.Workspace, args.Path)
		data, err := m.ReadFile(path)
		if err != nil {
			return tools.Result{}, err
		}
		return tools.Result{Content: tools.TruncateHead(string(data))}, nil
	})
}
