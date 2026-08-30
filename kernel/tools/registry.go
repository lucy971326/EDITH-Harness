package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	validator "github.com/santhosh-tekuri/jsonschema/v6"
)

// 活对象。挂在 Host 的 tools 键上的工具登记处。
type Registry struct {
	mu      sync.RWMutex
	entries map[string]entry
}

// 数据。一条登记工具及其已编译的校验规则。
type entry struct {
	tool   Tool
	schema *validator.Schema
}

// NewRegistry 造一张空的工具登记处。
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]entry)}
}

// Register 填入一条工具，并返回幂等注销函数。
func (r *Registry) Register(tool Tool) (func(), error) {
	if tool == nil {
		return nil, fmt.Errorf("tools: register nil tool")
	}

	definition := tool.Definition()
	if definition.Name == "" {
		return nil, fmt.Errorf("tools: register empty name")
	}
	if definition.Description == "" {
		return nil, fmt.Errorf("tools: register %q with empty description", definition.Name)
	}

	schema, err := compileSchema(definition.Name, definition.InputSchema)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[definition.Name]; exists {
		return nil, fmt.Errorf("tools: %q already registered", definition.Name)
	}
	r.entries[definition.Name] = entry{tool: tool, schema: schema}

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			delete(r.entries, definition.Name)
		})
	}, nil
}

// Definitions 按 Allow 的顺序取出本轮可交给模型的工具定义。
func (r *Registry) Definitions(allow []string) ([]Definition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]Definition, 0, len(allow))
	seen := make(map[string]struct{}, len(allow))
	for _, name := range allow {
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("tools: allowed tool %q appears more than once", name)
		}
		seen[name] = struct{}{}

		entry, ok := r.entries[name]
		if !ok {
			return nil, fmt.Errorf("tools: allowed tool %q is not registered", name)
		}
		definitions = append(definitions, entry.tool.Definition())
	}
	return definitions, nil
}

// Call 校验本轮权限和模型参数后，调用指定工具。
func (r *Registry) Call(ctx context.Context, call Call) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if !allowed(call.Name, call.Allow) {
		return toolError(fmt.Errorf("tool %q is not allowed for this run", call.Name)), nil
	}

	r.mu.RLock()
	entry, ok := r.entries[call.Name]
	r.mu.RUnlock()
	if !ok {
		return toolError(fmt.Errorf("tool %q is not registered", call.Name)), nil
	}

	var arguments any
	err := json.Unmarshal(call.Arguments, &arguments)
	if err != nil {
		return toolError(fmt.Errorf("tool %q arguments are not valid JSON: %w", call.Name, err)), nil
	}
	err = entry.schema.Validate(arguments)
	if err != nil {
		return toolError(fmt.Errorf("tool %q arguments do not match its schema: %w", call.Name, err)), nil
	}

	result, err := entry.tool.call(ctx, call, call.Arguments)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Result{}, ctxErr
	}
	if err != nil {
		return toolError(err), nil
	}
	return result, nil
}

func allowed(name string, allow []string) bool {
	for _, allowed := range allow {
		if name == allowed {
			return true
		}
	}
	return false
}

func toolError(err error) Result {
	return Result{Content: err.Error(), IsError: true}
}
