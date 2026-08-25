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

// Start 领取工具登记处并登记 write_file；文件能力在执行时从会话作用域领取。
func (Plugin) Start(app *core.App) error {
	registry, err := tools.Get(app)
	if err != nil {
		return fmt.Errorf("领取 tools 能力失败：%w", err)
	}

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
		Execute: writeFile{}.execute,
	})
	if err != nil {
		return fmt.Errorf("登记 write_file 失败：%w", err)
	}
	return nil
}

// writeFile 是从当前会话领取文件能力的工具实现。
type writeFile struct{}

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// execute 读取 JSON 参数并写文件，返回给模型的结果文字。
func (writeFile) execute(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error) {
	store, err := workspacefiles.Get(scope)
	if err != nil {
		return "", fmt.Errorf("领取 files 能力失败：%w", err)
	}
	var input writeFileInput
	err = json.Unmarshal(arguments, &input)
	if err != nil {
		return "", fmt.Errorf("write_file 参数不是合法 JSON：%w", err)
	}
	if input.Path == "" {
		return "", fmt.Errorf("write_file 缺少 path")
	}

	err = store.Write(ctx, input.Path, []byte(input.Content))
	if err != nil {
		return "", fmt.Errorf("写文件 %s 失败：%w", input.Path, err)
	}
	return fmt.Sprintf("已写入文件：%s", input.Path), nil
}
