package appserver

import (
	"context"
	"encoding/json"
	"errors"

	validator "github.com/santhosh-tekuri/jsonschema/v6"
)

// 活对象。保存类型化处理函数与组装时编译的输入、输出校验规则。
type boundMethod[Input, Output any] struct {
	handler      func(context.Context, Input) (Output, error)
	inputSchema  *validator.Schema
	outputSchema *validator.Schema
}

// Call 完整执行一次接口调用；校验与 JSON 转换只在这里处理。
func (m *boundMethod[Input, Output]) Call(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	// 输入检查：不符合契约的参数不能进入业务。
	inputValue, err := decodeJSON(raw)
	if err != nil {
		return nil, &Error{CodeInvalidParams, "input does not match contract", err}
	}
	err = m.inputSchema.Validate(inputValue)
	if err != nil {
		return nil, &Error{CodeInvalidParams, "input does not match contract", err}
	}

	// 解码：JSON 参数转成处理函数需要的 Go 类型。
	var input Input
	err = json.Unmarshal(raw, &input)
	if err != nil {
		return nil, &Error{CodeInvalidParams, "input cannot be decoded", err}
	}

	// 调用业务：保留公开错误，其余错误归为内部错误。
	output, err := m.handler(ctx, input)
	if err != nil {
		var publicError *Error
		if errors.As(err, &publicError) {
			return nil, publicError
		}
		return nil, &Error{CodeInternal, "handler failed", err}
	}

	// 编码与输出检查：业务结果也必须满足对外契约。
	encoded, err := json.Marshal(output)
	if err != nil {
		return nil, &Error{CodeInternal, "output does not match contract", err}
	}
	outputValue, err := decodeJSON(encoded)
	if err != nil {
		return nil, &Error{CodeInternal, "output does not match contract", err}
	}
	err = m.outputSchema.Validate(outputValue)
	if err != nil {
		return nil, &Error{CodeInternal, "output does not match contract", err}
	}
	return encoded, nil
}
