// Package files 提供文件工具插件。
package files

import (
	"context"
	"encoding/json"
	"fmt"

	"harness/chat"
	"harness/core"
	"harness/tools"
	workspacefiles "harness/workspace/files"
)

// Plugin 把文件工具登记进模型的工具菜单。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "tool-files"
}

// Start 领取 files 和 tools，登记 write_file。
func (Plugin) Start(app *core.App) error {
	store, err := workspacefiles.Get(app)
	if err != nil {
		return fmt.Errorf("领取 files 能力失败：%w", err)
	}
	registry, err := tools.Get(app)
	if err != nil {
		return fmt.Errorf("领取 tools 能力失败：%w", err)
	}

	writer := writeFile{store: store}
	err = registry.Register(tools.Tool{
		Schema: chat.ToolSchema{
			Name:        "write_file",
			Description: "把文本写到工作目录下的文件。",
			Parameters: []byte(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "相对于工作目录的文件路径"},
    "content": {"type": "string", "description": "要写入的文本"}
  },
  "required": ["path", "content"],
  "additionalProperties": false
}`),
		},
		Execute: writer.execute,
	})
	if err != nil {
		return fmt.Errorf("登记 write_file 失败：%w", err)
	}
	return nil
}

// writeFile 组合文件能力和 write_file 的执行方法。
type writeFile struct {
	store workspacefiles.Store // 底层文件能力
}

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// execute 读取 JSON 参数并写文件，返回给模型的结果文字。
func (w writeFile) execute(ctx context.Context, arguments json.RawMessage) (string, error) {
	var input writeFileInput
	err := json.Unmarshal(arguments, &input)
	if err != nil {
		return "", fmt.Errorf("write_file 参数不是合法 JSON：%w", err)
	}
	if input.Path == "" {
		return "", fmt.Errorf("write_file 缺少 path")
	}

	err = w.store.Write(ctx, input.Path, []byte(input.Content))
	if err != nil {
		return "", fmt.Errorf("写文件 %s 失败：%w", input.Path, err)
	}
	return fmt.Sprintf("已写入文件：%s", input.Path), nil
}
