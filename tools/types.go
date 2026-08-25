// Package tools 管模型的工具：一张两层登记表（全局 + 按会话遮蔽），
// 和一条九步流水线——顺序从此冻结，改顺序 = 破坏性变更。
package tools

import (
	"context"
	"encoding/json"

	"harness/chat"
	"harness/core"
)

// 流水线上的三个控制位（挂链用，先注册的在外层）。
const (
	PreExecute  = "tools/pre-execute"  // 前置检查：放行 / 拒 / 要问人
	Execute     = "tools/execute"      // 环绕：超时、重试挂这，本体在链尾
	PostExecute = "tools/post-execute" // 后置：可改结果、附话
	Guard       = "tools/guard"        // 守卫：只能拒（返回拒因），不能翻案
)

// Tool 是登记处里的一条工具：说明书 + 执行本体。
type Tool struct {
	Schema chat.ToolSchema
	// Execute 执行本体：拿到会话作用域和参数原文，返回给模型看的文字。
	// 尽量是插件结构体的方法值（有状态），别用匿名闭包藏状态。
	Execute func(ctx context.Context, scope *core.App, arguments json.RawMessage) (string, error)
}

// Call 是一次工具调用的全部信息。参数记了账就不许改——这里是值，链上也改不动。
type Call struct {
	ID       string          // 这次调用的编号（模型给的）
	Name     string          // 工具名
	Argument json.RawMessage // 参数原文，原样透传
	ScopeID  string          // 在哪段会话发起的，空 = 全局
}

// Result 是工具调用的终局。
type Result struct {
	Output string // 给模型看的
	Status string // "success" / "failed" / "skipped" / "unknown"
}

// Result 的四种状态。前三种会落成账上的 tool/result；
// unknown 特殊：表示"已开跑、结果不明、没记终局"，账上留着 start 无 result（M4 恢复时待裁决）。
const (
	ResultUnknown = "unknown"
)

// Decision 是前置检查或问人的结论。
type Decision struct {
	Kind   string // "allow" 放行 / "deny" 拒 / "ask" 要问人
	Reason string // 拒或要问人的原因，会转给模型看
}

// Decision 的三种结论。
const (
	Allow = "allow"
	Deny  = "deny"
	Ask   = "ask"
)

// PreCall 是前置检查链的输入：这次调用 + 它的说明书。
type PreCall struct {
	Call Call
	Tool chat.ToolSchema
}

// PostCall 是后置链的输入：这次调用 + 草拟的终局，链可以改 Output。
type PostCall struct {
	Call   Call
	Result Result
}

// Approver 是问人的能力：一次要问人的调用进来，一个决定出去。
// 挂在能力名 "approval" 下；前置链说"要问人"而没人挂这个能力 = 拒（问了没人答）。
type Approver interface {
	Approve(call Call) Decision
}

// Outcome 是环绕链和本体之间的交接物：超时、重试插件看得到 err 才好重试。
type Outcome struct {
	Output string
	Err    error
}
