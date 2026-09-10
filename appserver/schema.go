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

// compileMethod 为登记准备输入、输出校验规则。
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
	// 顶层只接受结构体，避免 null、任意 map 或标量成为接口输入输出。
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
