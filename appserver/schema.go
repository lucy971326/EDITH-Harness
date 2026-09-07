package appserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	reflector "github.com/invopop/jsonschema"
	validator "github.com/santhosh-tekuri/jsonschema/v6"
)

// Describe 不启动产品；从方法声明生成并编译同一份运行时契约。
func Describe[I, O any](method Method[I, O]) (Definition, error) {
	definition, _, _, err := describe(method)
	return definition, err
}

func describe[I, O any](method Method[I, O]) (Definition, *validator.Schema, *validator.Schema, error) {
	if strings.TrimSpace(method.Name) == "" || strings.TrimSpace(method.Name) != method.Name {
		return Definition{}, nil, nil, fmt.Errorf("appserver: invalid method name %q", method.Name)
	}
	input, in, err := schemaFor(reflect.TypeFor[I]())
	if err != nil {
		return Definition{}, nil, nil, fmt.Errorf("appserver: %s input: %w", method.Name, err)
	}
	output, out, err := schemaFor(reflect.TypeFor[O]())
	if err != nil {
		return Definition{}, nil, nil, fmt.Errorf("appserver: %s output: %w", method.Name, err)
	}
	return Definition{method.Name, method.Description, input, output}, in, out, nil
}

func schemaFor(typ reflect.Type) (json.RawMessage, *validator.Schema, error) {
	// 顶层只接受结构体，避免 null、任意 map 或标量成为接口输入输出。
	if typ.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("contract must be a struct, got %v", typ)
	}
	r := reflector.Reflector{Anonymous: true, ExpandedStruct: true}
	raw, err := json.Marshal(r.ReflectFromType(typ))
	if err != nil {
		return nil, nil, err
	}
	value, err := decodeJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	c := validator.NewCompiler()
	c.AssertFormat()
	err = c.AddResource("urn:harness:contract", value)
	if err != nil {
		return nil, nil, err
	}
	schema, err := c.Compile("urn:harness:contract")
	return raw, schema, err
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
