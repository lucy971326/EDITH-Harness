package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// 契约。具体工具拿到已校验参数后的执行函数。
type Handler[Args any] func(ctx context.Context, call Call, args Args) (Result, error)

// New 用参数结构体和 tag 造一条工具。Schema 由结构体自动生成。
func New[Args any](name string, description string, handler Handler[Args]) Tool {
	return &tool[Args]{
		definition: Definition{
			Name:        name,
			Description: description,
			InputSchema: schemaFor[Args](),
		},
		handler: handler,
	}
}

// 活对象。一条带强类型参数转换的工具。
type tool[Args any] struct {
	definition Definition
	handler    Handler[Args]
}

func (t *tool[Args]) Definition() Definition {
	return t.definition
}

func (t *tool[Args]) call(ctx context.Context, call Call, arguments json.RawMessage) (Result, error) {
	var args Args
	err := json.Unmarshal(arguments, &args)
	if err != nil {
		return Result{}, fmt.Errorf("tools: decode %q arguments: %w", t.definition.Name, err)
	}
	return t.handler(ctx, call, args)
}
