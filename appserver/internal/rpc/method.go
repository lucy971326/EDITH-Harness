package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	reflector "github.com/invopop/jsonschema"
	validator "github.com/santhosh-tekuri/jsonschema/v6"
)

// 契约。PreparedCall 是已经校验输入并保留执行顺序的一次调用。
type PreparedCall func() (json.RawMessage, error)

// 契约。Method 是已绑定运行时校验、类型转换与调度方式的处理函数。
type Method func(context.Context, json.RawMessage) PreparedCall

type boundMethod[Input, Output any] struct {
	handler      func(context.Context, Input) (Output, error)
	inputSchema  *validator.Schema
	outputSchema *validator.Schema
	scheduler    *Scheduler
	sessionKey   func(Input) string
}

// Prepare 先校验输入并保留 Session 顺序，再返回等待完整结果的调用。
func (m *boundMethod[Input, Output]) Prepare(ctx context.Context, raw json.RawMessage) PreparedCall {
	inputValue, err := decodeJSON(raw)
	if err != nil {
		return failedCall(&Error{Code: CodeInvalidParams, Message: "input does not match contract", Cause: err})
	}
	err = m.inputSchema.Validate(inputValue)
	if err != nil {
		return failedCall(&Error{Code: CodeInvalidParams, Message: "input does not match contract", Cause: err})
	}

	var input Input
	err = json.Unmarshal(raw, &input)
	if err != nil {
		return failedCall(&Error{Code: CodeInvalidParams, Message: "input cannot be decoded", Cause: err})
	}

	invoke := func() (json.RawMessage, error) {
		output, callErr := m.handler(ctx, input)
		if callErr != nil {
			return nil, publicCallError(callErr)
		}

		encoded, encodeErr := json.Marshal(output)
		if encodeErr != nil {
			return nil, &Error{Code: CodeInternal, Message: "output does not match contract", Cause: encodeErr}
		}
		outputValue, decodeErr := decodeJSON(encoded)
		if decodeErr != nil {
			return nil, &Error{Code: CodeInternal, Message: "output does not match contract", Cause: decodeErr}
		}
		validateErr := m.outputSchema.Validate(outputValue)
		if validateErr != nil {
			return nil, &Error{Code: CodeInternal, Message: "output does not match contract", Cause: validateErr}
		}
		return encoded, nil
	}
	if m.scheduler == nil {
		return invoke
	}

	var result json.RawMessage
	wait := m.scheduler.Schedule(ctx, m.sessionKey(input), func() error {
		var callErr error
		result, callErr = invoke()
		return callErr
	})
	return func() (json.RawMessage, error) {
		callErr := wait()
		if callErr != nil {
			return nil, publicCallError(callErr)
		}
		return result, nil
	}
}

func failedCall(err error) PreparedCall {
	return func() (json.RawMessage, error) {
		return nil, err
	}
}

func publicCallError(err error) error {
	var publicError *Error
	if errors.As(err, &publicError) {
		return publicError
	}
	return &Error{Code: CodeInternal, Message: "handler failed", Cause: err}
}

func bind[Input, Output any](name string, handler func(context.Context, Input) (Output, error), scheduler *Scheduler, sessionKey func(Input) string) (Method, error) {
	if handler == nil {
		return nil, fmt.Errorf("appserver: nil handler")
	}
	inputSchema, outputSchema, err := compileMethod[Input, Output](name)
	if err != nil {
		return nil, err
	}
	method := &boundMethod[Input, Output]{
		handler:      handler,
		inputSchema:  inputSchema,
		outputSchema: outputSchema,
		scheduler:    scheduler,
		sessionKey:   sessionKey,
	}
	return method.Prepare, nil
}

func compileMethod[Input, Output any](name string) (*validator.Schema, *validator.Schema, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name {
		return nil, nil, fmt.Errorf("appserver: invalid method name %q", name)
	}
	if strings.HasPrefix(name, "rpc.") || name == "initialize" || name == "server/unsubscribe" {
		return nil, nil, fmt.Errorf("appserver: reserved method name %q", name)
	}
	inputSchema, err := schemaFor(reflect.TypeFor[Input]())
	if err != nil {
		return nil, nil, fmt.Errorf("appserver: %s input: %w", name, err)
	}
	outputSchema, err := schemaFor(reflect.TypeFor[Output]())
	if err != nil {
		return nil, nil, fmt.Errorf("appserver: %s output: %w", name, err)
	}
	return inputSchema, outputSchema, nil
}

func schemaFor(typ reflect.Type) (*validator.Schema, error) {
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("contract must be a struct, got %v", typ)
	}
	schemaReflector := reflector.Reflector{Anonymous: true, ExpandedStruct: true}
	raw, err := json.Marshal(schemaReflector.ReflectFromType(typ))
	if err != nil {
		return nil, err
	}
	value, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	compiler := validator.NewCompiler()
	compiler.AssertFormat()
	err = compiler.AddResource("urn:harness:contract", value)
	if err != nil {
		return nil, err
	}
	return compiler.Compile("urn:harness:contract")
}

func decodeJSON(raw []byte) (any, error) {
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	err := decoder.Decode(&value)
	return value, err
}
