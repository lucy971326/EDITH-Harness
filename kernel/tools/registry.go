package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	validator "github.com/santhosh-tekuri/jsonschema/v6"
)

// 活对象。挂在 Host 的 tools 键上的工具登记处。
type Registry struct {
	mu        sync.RWMutex
	entries   map[string]entry
	providers []providerEntry
	owners    map[string]string
}

// Defaults 按名称稳定列出默认 Agent 自动选择的固定工具。
func (r *Registry) Defaults() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.entries))
	for _, entry := range r.entries {
		out = append(out, entry.tool.Definition())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// List 按名称稳定列出 Agent 设置可选择的全部工具定义。
func (r *Registry) List() []Definition {
	out := r.Defaults()
	r.mu.RLock()
	providers := append([]providerEntry(nil), r.providers...)
	r.mu.RUnlock()
	for _, entry := range providers {
		out = append(out, entry.provider.Choices()...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// 数据。一条登记工具及其已编译的校验规则。
type entry struct {
	tool   Tool
	schema *validator.Schema
}

// 数据。一条已登记动态来源及其稳定身份。
type providerEntry struct {
	name     string
	provider Provider
}

// 数据。一条动态 Tool 及其参数规则和调用来源。
type dynamicEntry struct {
	definition Definition
	schema     *validator.Schema
	provider   Provider
}

// NewRegistry 造一张空的工具登记处。
func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]entry),
		owners:  make(map[string]string),
	}
}

// Register 填入一条启动时固定的工具。
func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("tools: register nil tool")
	}

	definition := tool.Definition()
	if definition.Name == "" {
		return fmt.Errorf("tools: register empty name")
	}
	if definition.Description == "" {
		return fmt.Errorf("tools: register %q with empty description", definition.Name)
	}

	schema, err := compileSchema(definition.Name, definition.InputSchema)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if owner, exists := r.owners[definition.Name]; exists {
		return fmt.Errorf("tools: %q already registered by %s", definition.Name, owner)
	}
	if _, exists := r.entries[definition.Name]; exists {
		return fmt.Errorf("tools: %q already registered", definition.Name)
	}
	r.entries[definition.Name] = entry{tool: tool, schema: schema}
	return nil
}

// RegisterProvider 填入一个启动时固定的动态 Tool 来源。
func (r *Registry) RegisterProvider(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("tools: register nil provider")
	}
	name := provider.Name()
	if name == "" {
		return fmt.Errorf("tools: register provider with empty name")
	}
	choices := provider.Choices()
	seen := make(map[string]struct{}, len(choices))
	for _, definition := range choices {
		if _, duplicate := seen[definition.Name]; duplicate {
			return fmt.Errorf("tools: provider %q returned duplicate %q", name, definition.Name)
		}
		seen[definition.Name] = struct{}{}
		_, err := validateDefinition(definition)
		if err != nil {
			return fmt.Errorf("tools: provider %q: %w", name, err)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.providers {
		if entry.name == name {
			return fmt.Errorf("tools: provider %q already registered", name)
		}
	}
	for toolName := range seen {
		if _, exists := r.entries[toolName]; exists {
			return fmt.Errorf("tools: %q already registered", toolName)
		}
		if owner, exists := r.owners[toolName]; exists {
			return fmt.Errorf("tools: %q already registered by %s", toolName, owner)
		}
	}
	r.providers = append(r.providers, providerEntry{name: name, provider: provider})
	for toolName := range seen {
		r.owners[toolName] = name
	}
	return nil
}

// Prepare 合并 Agent 已选 Tool 和当前工作区自动 Tool。
func (r *Registry) Prepare(ctx context.Context, workspace string, selected []string) (Prepared, error) {
	_, err := r.definitions(ctx, "", selected)
	if err != nil {
		return Prepared{}, err
	}
	_, snapshots, err := r.dynamic(ctx, workspace)
	if err != nil {
		return Prepared{}, err
	}
	names := append([]string(nil), selected...)
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		seen[name] = struct{}{}
	}
	var instructions []Instruction
	for _, snapshot := range snapshots {
		for _, name := range snapshot.Automatic {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
		instructions = append(instructions, snapshot.Instructions...)
	}
	_, err = r.definitions(ctx, workspace, names)
	if err != nil {
		return Prepared{}, err
	}
	activeInstructions := instructions[:0]
	for _, instruction := range instructions {
		if intersects(instruction.ToolNames, seen) {
			activeInstructions = append(activeInstructions, instruction)
		}
	}
	return Prepared{Names: names, Instructions: activeInstructions}, nil
}

// Definitions 按 Allow 的顺序取出本轮可交给模型的工具定义。
func (r *Registry) Definitions(ctx context.Context, workspace string, allow []string) ([]Definition, error) {
	return r.definitions(ctx, workspace, allow)
}

func (r *Registry) definitions(ctx context.Context, workspace string, allow []string) ([]Definition, error) {
	r.mu.RLock()
	static := make(map[string]entry, len(r.entries))
	for name, entry := range r.entries {
		static[name] = entry
	}
	r.mu.RUnlock()
	dynamic, _, err := r.dynamic(ctx, workspace)
	if err != nil {
		return nil, err
	}

	definitions := make([]Definition, 0, len(allow))
	seen := make(map[string]struct{}, len(allow))
	for _, name := range allow {
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("tools: allowed tool %q appears more than once", name)
		}
		seen[name] = struct{}{}

		if entry, ok := static[name]; ok {
			definitions = append(definitions, entry.tool.Definition())
			continue
		}
		entry, ok := dynamic[name]
		if !ok {
			return nil, fmt.Errorf("tools: allowed tool %q is not registered", name)
		}
		definitions = append(definitions, entry.definition)
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
	staticEntry, static := r.entries[call.Name]
	r.mu.RUnlock()
	var schema *validator.Schema
	var provider Provider
	if static {
		schema = staticEntry.schema
	} else {
		dynamic, _, err := r.dynamic(ctx, call.Workspace)
		if err != nil {
			return toolError(err), nil
		}
		entry, ok := dynamic[call.Name]
		if !ok {
			return toolError(fmt.Errorf("tool %q is not registered", call.Name)), nil
		}
		schema = entry.schema
		provider = entry.provider
	}

	var arguments any
	err := json.Unmarshal(call.Arguments, &arguments)
	if err != nil {
		return toolError(fmt.Errorf("tool %q arguments are not valid JSON: %w", call.Name, err)), nil
	}
	err = schema.Validate(arguments)
	if err != nil {
		return toolError(fmt.Errorf("tool %q arguments do not match its schema: %w", call.Name, err)), nil
	}

	var result Result
	if static {
		result, err = staticEntry.tool.call(ctx, call, call.Arguments)
	} else {
		result, err = provider.Call(ctx, call)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Result{}, ctxErr
	}
	if err != nil {
		return toolError(err), nil
	}
	return result, nil
}

func (r *Registry) dynamic(ctx context.Context, workspace string) (map[string]dynamicEntry, []Snapshot, error) {
	r.mu.RLock()
	providers := append([]providerEntry(nil), r.providers...)
	staticNames := make(map[string]struct{}, len(r.entries))
	for name := range r.entries {
		staticNames[name] = struct{}{}
	}
	r.mu.RUnlock()

	all := make(map[string]dynamicEntry)
	snapshots := make([]Snapshot, 0, len(providers))
	for _, provider := range providers {
		snapshot, err := provider.provider.Snapshot(ctx, workspace)
		if err != nil {
			return nil, nil, fmt.Errorf("tools: provider %q: %w", provider.name, err)
		}
		seen := make(map[string]struct{}, len(snapshot.Definitions))
		for _, definition := range snapshot.Definitions {
			if _, duplicate := seen[definition.Name]; duplicate {
				return nil, nil, fmt.Errorf("tools: provider %q returned duplicate %q", provider.name, definition.Name)
			}
			seen[definition.Name] = struct{}{}
			if _, exists := staticNames[definition.Name]; exists {
				return nil, nil, fmt.Errorf("tools: provider %q conflicts with registered tool %q", provider.name, definition.Name)
			}
			if previous, exists := all[definition.Name]; exists {
				return nil, nil, fmt.Errorf("tools: tool %q is provided by %q and %q", definition.Name, previous.provider.Name(), provider.name)
			}
			schema, err := validateDefinition(definition)
			if err != nil {
				return nil, nil, fmt.Errorf("tools: provider %q: %w", provider.name, err)
			}
			all[definition.Name] = dynamicEntry{definition: definition, schema: schema, provider: provider.provider}
		}
		for _, name := range snapshot.Automatic {
			if _, exists := seen[name]; !exists {
				return nil, nil, fmt.Errorf("tools: provider %q marked unknown tool %q automatic", provider.name, name)
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return all, snapshots, nil
}

func validateDefinition(definition Definition) (*validator.Schema, error) {
	if definition.Name == "" {
		return nil, fmt.Errorf("register empty name")
	}
	if definition.Description == "" {
		return nil, fmt.Errorf("register %q with empty description", definition.Name)
	}
	return compileSchema(definition.Name, definition.InputSchema)
}

func intersects(names []string, active map[string]struct{}) bool {
	for _, name := range names {
		if _, ok := active[name]; ok {
			return true
		}
	}
	return false
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
