package edit

import (
	"context"
	"fmt"
	"strings"

	"harness/kernel/machine"
	"harness/kernel/tools"
)

// 数据。edit 工具的模型参数。
type Args struct {
	Path    string `json:"path" jsonschema:"minLength=1,description=Path to the file to edit. Relative paths start at the workspace."`
	OldText string `json:"oldText" jsonschema:"minLength=1,description=Exact text that appears once in the file."`
	NewText string `json:"newText" jsonschema:"description=Replacement text."`
}

func newTool(m machine.Machine) tools.Tool {
	return tools.New("edit", "Replace one exact, uniquely matched text block in a file.", func(ctx context.Context, call tools.Call, args Args) (tools.Result, error) {
		if err := ctx.Err(); err != nil {
			return tools.Result{}, err
		}
		path := m.ResolvePath(call.Workspace, args.Path)
		data, err := m.ReadFile(path)
		if err != nil {
			return tools.Result{}, err
		}

		content := string(data)
		count := strings.Count(content, args.OldText)
		if count == 0 {
			return tools.Result{}, fmt.Errorf("edit %q: oldText was not found", args.Path)
		}
		if count > 1 {
			return tools.Result{}, fmt.Errorf("edit %q: oldText appears %d times; provide more context", args.Path, count)
		}

		updated := strings.Replace(content, args.OldText, args.NewText, 1)
		if err := ctx.Err(); err != nil {
			return tools.Result{}, err
		}
		err = m.WriteFile(path, []byte(updated))
		if err != nil {
			return tools.Result{}, err
		}
		return tools.Result{Content: "replaced 1 block in " + args.Path}, nil
	})
}
