package write

import (
	"context"
	"fmt"

	"harness/kernel/machine"
	"harness/kernel/tools"
)

// 数据。write 工具的模型参数。
type Args struct {
	Path    string `json:"path" jsonschema:"minLength=1,description=Path to the file to write. Relative paths start at the workspace."`
	Content string `json:"content" jsonschema:"description=Full content that replaces the file."`
}

func newTool(m machine.Machine) tools.Tool {
	return tools.New("write", "Write full text to a file. Creates parent directories and overwrites existing content.", func(ctx context.Context, call tools.Call, args Args) (tools.Result, error) {
		if err := ctx.Err(); err != nil {
			return tools.Result{}, err
		}
		path := m.ResolvePath(call.Workspace, args.Path)
		err := m.WriteFile(path, []byte(args.Content))
		if err != nil {
			return tools.Result{}, err
		}
		return tools.Result{Content: fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path)}, nil
	})
}
