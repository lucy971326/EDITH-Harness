package tools

import (
	"encoding/json"
	"fmt"

	invopop "github.com/invopop/jsonschema"
	validator "github.com/santhosh-tekuri/jsonschema/v6"
)

func schemaFor[Args any]() json.RawMessage {
	reflector := invopop.Reflector{
		Anonymous:                 true,
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	schema := reflector.Reflect(new(Args))
	data, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("tools: marshal generated schema: %v", err))
	}
	return data
}

func compileSchema(name string, input json.RawMessage) (*validator.Schema, error) {
	if !json.Valid(input) {
		return nil, fmt.Errorf("tools: tool %q has invalid generated schema", name)
	}
	var resource any
	err := json.Unmarshal(input, &resource)
	if err != nil {
		return nil, fmt.Errorf("tools: decode generated schema for %q: %w", name, err)
	}

	path := "tool-" + name + ".json"
	compiler := validator.NewCompiler()
	err = compiler.AddResource(path, resource)
	if err != nil {
		return nil, fmt.Errorf("tools: add schema for %q: %w", name, err)
	}

	schema, err := compiler.Compile(path)
	if err != nil {
		return nil, fmt.Errorf("tools: compile schema for %q: %w", name, err)
	}
	return schema, nil
}
