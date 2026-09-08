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

// compileMethod 为登记准备运行时目录与输入、输出校验规则，不生成 TS。
func compileMethod[Input, Output any](method Method[Input, Output]) (Definition, *validator.Schema, *validator.Schema, error) {
	if strings.TrimSpace(method.Name) == "" || strings.TrimSpace(method.Name) != method.Name {
		return Definition{}, nil, nil, fmt.Errorf("appserver: invalid method name %q", method.Name)
	}
	inputJSON, inputSchema, err := schemaFor(reflect.TypeFor[Input]())
	if err != nil {
		return Definition{}, nil, nil, fmt.Errorf("appserver: %s input: %w", method.Name, err)
	}
	outputJSON, outputSchema, err := schemaFor(reflect.TypeFor[Output]())
	if err != nil {
		return Definition{}, nil, nil, fmt.Errorf("appserver: %s output: %w", method.Name, err)
	}
	definition := Definition{
		Name:         method.Name,
		Description:  method.Description,
		InputSchema:  inputJSON,
		OutputSchema: outputJSON,
	}
	return definition, inputSchema, outputSchema, nil
}

func schemaFor(typ reflect.Type) (json.RawMessage, *validator.Schema, error) {
	// 顶层只接受结构体，避免 null、任意 map 或标量成为接口输入输出。
	if typ.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("contract must be a struct, got %v", typ)
	}
	schemaReflector := reflector.Reflector{Anonymous: true, ExpandedStruct: true}
	raw, err := json.Marshal(schemaReflector.ReflectFromType(typ))
	if err != nil {
		return nil, nil, err
	}
	value, err := decodeJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	compiler := validator.NewCompiler()
	compiler.AssertFormat()
	err = compiler.AddResource("urn:harness:contract", value)
	if err != nil {
		return nil, nil, err
	}
	schema, err := compiler.Compile("urn:harness:contract")
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
