// Package session 是账本：记住发生过的一切，让所有人从同一份记录里各取所需。
// 规矩极硬：只增不改、编号连续、记账时验、要么整条要么没有。
package session

import (
	"encoding/json"
	"time"

	"harness/chat"
)

// Event 是账本里的一笔。
type Event struct {
	Kind          string          // 事的种类
	Seq           int             // 第几笔，从 1 起，必须连续
	Time          int64           // 记账时刻（Unix 毫秒）
	Data          json.RawMessage // 内容，记账时编码并锁死
	Replaces      []int           // 摘要专用："我取代了这几笔"——压缩全靠它
	SkipIfUnknown bool            // 写这笔的插件将来没了：true=读到跳过，false=整本账拒读
}

// Header 是账本封面：整本账一个格式版本。
type Header struct {
	FormatVersion  int       // 格式版本，当前 4
	ID             string    // 内部会话号，不拿标题充当身份
	Title          string    // 给用户看的标题，不要求全局唯一
	CreatedAt      time.Time // 会话创建时间
	ProjectID      string    // 所属项目的稳定身份
	ProjectRoot    string    // 创建时锁定的项目根目录
	PresetID       string    // 使用的 Agent 模式
	PresetRevision int       // 锁定的模式版本
}

// 内核事件的种类。全部走专用记账方法，公开 AppendEvent 见到它们直接拒绝。
const (
	KindUserMessage    = "user/message"     // 用户说的话
	KindChunk          = "chunk"            // 模型吐的字（流式素材，说没说不算数，看定稿）
	KindAssistantFinal = "assistant/final"  // 模型一段话的定稿：收编引用的字，带用量和是否被打断
	KindToolCall       = "tool/call"        // 模型要调工具
	KindToolStart      = "tool/start"       // 工具真的开始执行了（副作用窗口的边界）
	KindToolResult     = "tool/result"      // 工具的回话（成功/失败/跳过）
	KindTurnStart      = "turn/start"       // 一轮开始
	KindTurnEnd        = "turn/end"         // 一轮结束
	KindStepStart      = "step/start"       // 一步开始
	KindStepEnd        = "step/end"         // 一步结束
	KindDeliver        = "inbox/deliver"    // 消息进收件箱（只入账，模型还看不见）
	KindClaim          = "inbox/claim"      // 从收件箱领出（这时才变成 user/message）
	KindSnapshot       = "request/snapshot" // 当年发给模型的那份请求的存档
	KindSummary        = "summary"          // 压缩摘要：取代一堆旧账，模型看它
	KindModelSelected  = "model/selected"   // 会话切换到哪个模型组合
)

// 各内核事件的内容结构。字段只加不改，改了就是格式版本的事。

type UserMessageData struct {
	Text string
}

type ChunkData struct {
	Delta string // 这一小段新增的字
}

type AssistantFinalData struct {
	Text        string     // 这段话的完整定稿
	Thinking    string     // 服务商返回的隐藏思考；续聊时交还给同一服务商
	Interrupted bool       // true = 被打断的半句（已收到的部分固化）
	Usage       chat.Usage // 这段话的用量（花了多少 token）
}

type ToolCallData struct {
	ID       string
	Name     string
	Argument json.RawMessage
}

type ToolStartData struct {
	CallID string
}

type ToolResultData struct {
	CallID string
	Output string
	Status string // "success" / "failed" / "skipped"
}

type DeliverData struct {
	ID     string // 这份投递的编号，领出时对账
	Text   string
	Target string // 给谁："next-turn" 开新轮 / "next-step" 进当前轮下一步
}

type ClaimData struct {
	ID string
}

type SnapshotData struct {
	Request []byte // 当年发给模型的完整请求（llm.Request 的 JSON 原文）
}

type SummaryData struct {
	Text     string     // 摘要正文
	Provider string     // 谁写的摘要（服务商名），可空
	Model    string     // 哪个模型写的，可空
	Usage    chat.Usage // 写摘要花了多少 token
}

// ModelSelectedData 是一段会话某时刻选中的模型组合。
type ModelSelectedData struct {
	Provider string
	Model    string
	Thinking string
}

// 工具结果的三种状态。
const (
	ResultSuccess = "success"
	ResultFailed  = "failed"
	ResultSkipped = "skipped"
)

// now 返回当前 Unix 毫秒，单独成函数方便测试控制时间。
var now = func() int64 {
	return time.Now().UnixMilli()
}
