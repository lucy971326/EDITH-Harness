package session

import (
	"encoding/json"
	"fmt"
	"sync"
)

// eventFormat 是一种事件的格式说明：内容怎么解、要不要进模型历史。
type eventFormat struct {
	modelVisible  bool                               // 要不要变成模型的消息
	kernel        bool                               // 内核种类：公开 AppendEvent 拒绝，只走专用方法
	decode        func(json.RawMessage) (any, error) // JSON -> 结构体，兼作账数据的体检
	skipIfUnknown bool                               // 插件没了之后：跳过还是拒读（仅插件种类用）
}

// builtinFormats 是内核种类的格式表，session 是这些词的唯一主人。
var builtinFormats = map[string]eventFormat{
	KindUserMessage:    {modelVisible: true, kernel: true, decode: decodeAs(&UserMessageData{})},
	KindChunk:          {modelVisible: true, kernel: true, decode: decodeAs(&ChunkData{})},
	KindAssistantFinal: {modelVisible: true, kernel: true, decode: decodeAs(&AssistantFinalData{})},
	KindToolCall:       {modelVisible: true, kernel: true, decode: decodeAs(&ToolCallData{})},
	KindToolStart:      {modelVisible: false, kernel: true, decode: decodeAs(&ToolStartData{})},
	KindToolResult:     {modelVisible: true, kernel: true, decode: decodeAs(&ToolResultData{})},
	KindTurnStart:      {modelVisible: false, kernel: true, decode: decodeAs(&struct{}{})},
	KindTurnEnd:        {modelVisible: false, kernel: true, decode: decodeAs(&struct{}{})},
	KindStepStart:      {modelVisible: false, kernel: true, decode: decodeAs(&struct{}{})},
	KindStepEnd:        {modelVisible: false, kernel: true, decode: decodeAs(&struct{}{})},
	KindDeliver:        {modelVisible: false, kernel: true, decode: decodeAs(&DeliverData{})},
	KindClaim:          {modelVisible: false, kernel: true, decode: decodeAs(&ClaimData{})},
	KindSnapshot:       {modelVisible: false, kernel: true, decode: decodeAs(&SnapshotData{})},
	KindSummary:        {modelVisible: true, kernel: true, decode: decodeAs(&SummaryData{})},
}

// decodeAs 返回"把 JSON 解进这种结构体"的函数，解不动就是账数据坏了。
func decodeAs[T any](blank *T) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		value := new(T)
		err := json.Unmarshal(raw, value)
		if err != nil {
			return nil, err
		}
		return value, nil
	}
}

// formatTable 是一张完整格式表：内核种类 + 插件注册的种类。
type formatTable struct {
	mu      sync.RWMutex
	customs map[string]eventFormat
}

func newFormatTable() *formatTable {
	return &formatTable{customs: make(map[string]eventFormat)}
}

// lookup 查一种种类；第二返回值说认不认识。
func (t *formatTable) lookup(kind string) (eventFormat, bool) {
	if format, ok := builtinFormats[kind]; ok {
		return format, true
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	format, ok := t.customs[kind]
	return format, ok
}

// register 给插件注册自定义种类：默认不进模型历史。
// 种类名撞内核或撞别的插件都拒绝。
func (t *formatTable) register(kind string, skipIfUnknown bool) error {
	_, kernel := builtinFormats[kind]
	if kernel {
		return fmt.Errorf("种类 %s 是内核事件，不能注册", kind)
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	_, taken := t.customs[kind]
	if taken {
		return fmt.Errorf("种类 %s 已被别的插件注册", kind)
	}
	t.customs[kind] = eventFormat{kernel: false, skipIfUnknown: skipIfUnknown}
	return nil
}

// checkRead 读旧账时体检一笔：认识的种类数据必须能解；
// 不认识的按 SkipIfUnknown 定跳过或拒读。
func (t *formatTable) checkRead(event Event) error {
	format, known := t.lookup(event.Kind)
	if known {
		_, err := format.decode(event.Data)
		if err != nil {
			return fmt.Errorf("第 %d 笔（%s）数据坏了：%w", event.Seq, event.Kind, err)
		}
		return nil
	}
	if event.SkipIfUnknown {
		return nil
	}
	return fmt.Errorf("第 %d 笔的种类 %s 不认识，整本账拒读（宁严勿猜）", event.Seq, event.Kind)
}

// isKernel 种类是否内核所有。
func (t *formatTable) isKernel(kind string) bool {
	_, kernel := builtinFormats[kind]
	return kernel
}

// isDurable 哪些种类必须立刻真落盘：流式片段可以攒批，其余都是关键账。
func isDurable(kind string) bool {
	return kind != KindChunk
}
