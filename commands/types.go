package commands

import "context"

// Command 是登记处里的一条人类命令：发现材料和执行本体。
type Command struct {
	Name        string // 不带斜杠的小写名字
	Description string // 菜单里的一句话
	Group       string // 菜单分组；空则不分组
	Hint        string // 参数占位提示；空表示没有参数
	// Run 执行本体：拿到这次调用，返回给人看的结果。
	// 尽量是插件结构体的方法值，别用匿名闭包藏状态。
	Run func(ctx context.Context, invocation Invocation) Result
}

// Descriptor 是给界面看的命令说明书，不含执行本体。
type Descriptor struct {
	Name        string
	Description string
	Group       string
	Hint        string
}

// Invocation 是一次命令调用交给执行本体的材料。
type Invocation struct {
	Name      string // 解析出的命令名
	RawInput  string // 名字后面的原文，含分隔空白
	SessionID string // 当前会话；草稿为空
}

// Result 是命令的终局，只给人看，不进账本。
type Result struct {
	Kind string // "success" / "error"
	Text string // 界面提示；成功可空
}

// Result 的两种终局。
const (
	KindSuccess = "success"
	KindError   = "error"
)

// Parsed 是一行斜杠输入拆开后的名字和参数。
type Parsed struct {
	Name     string
	RawInput string
}
