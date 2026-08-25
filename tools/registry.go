package tools

import (
	"fmt"
	"sort"
	"sync"

	"harness/chat"
)

// Registry 是工具登记处：一张全局表 + 每段会话一张小表（同名遮蔽）。
// 查找先小表后大表；会话关闭后整张小表被丢掉。
type Registry struct {
	mu       sync.Mutex
	global   map[string]Tool
	perScope map[string]scopeTools
}

// scopeTools 是一段会话的工具小表；only=true 时名单外的全局工具也不可见。
type scopeTools struct {
	tools map[string]Tool
	only  bool
}

// NewRegistry 建一个空的登记处。
func NewRegistry() *Registry {
	return &Registry{
		global:   make(map[string]Tool),
		perScope: make(map[string]scopeTools),
	}
}

// Register 登记一个全局工具；同名重复登记是组装错误。
func (r *Registry) Register(tool Tool) error {
	return r.register("", tool)
}

// RegisterForScope 给某段会话登记专用工具，遮蔽同名全局工具。
func (r *Registry) RegisterForScope(scopeID string, tool Tool) error {
	if scopeID == "" {
		return fmt.Errorf("专用工具要写明会话作用域")
	}
	return r.register(scopeID, tool)
}

func (r *Registry) register(scopeID string, tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	table := r.tableFor(scopeID)
	name := tool.Schema.Name
	_, taken := table[name]
	if taken {
		return fmt.Errorf("工具 %s 在 %s 的表里已登记过", name, scopeName(scopeID))
	}
	table[name] = tool
	return nil
}

// Lookup 查工具：会话小表命中即遮蔽全局，查不到再看全局表。
func (r *Registry) Lookup(name string, scopeID string) (Tool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if scopeID != "" {
		table, exists := r.perScope[scopeID]
		tool, ok := table.tools[name]
		if ok {
			return tool, true
		}
		if exists && table.only {
			return Tool{}, false
		}
	}
	tool, ok := r.global[name]
	return tool, ok
}

// Schemas 列出一段会话能看见的全部工具说明书。
func (r *Registry) Schemas(scopeID string) []chat.ToolSchema {
	r.mu.Lock()
	defer r.mu.Unlock()

	if scopeID != "" {
		table, exists := r.perScope[scopeID]
		if exists && table.only {
			return schemasFrom(table.tools)
		}
	}
	seen := make(map[string]bool)
	schemas := schemasFrom(r.global)
	for _, schema := range schemas {
		seen[schema.Name] = true
	}
	if scopeID != "" {
		table, exists := r.perScope[scopeID]
		if exists {
			for name, tool := range table.tools {
				if !seen[name] {
					schemas = append(schemas, tool.Schema)
					seen[name] = true
					continue
				}
				// 遮蔽：换掉全局那份说明书
				for i, schema := range schemas {
					if schema.Name == name {
						schemas[i] = tool.Schema
					}
				}
			}
		}
	}
	return schemas
}

// SetAllowedForScope 给一段会话绑定明确的工具白名单。名单外的全局工具也不可见；
// 名单里的每个工具必须已由应用启动时登记。
func (r *Registry) SetAllowedForScope(scopeID string, names []string) error {
	if scopeID == "" {
		return fmt.Errorf("工具白名单必须写明会话作用域")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	table := make(map[string]Tool, len(names))
	for _, name := range names {
		tool, exists := r.global[name]
		if !exists {
			return fmt.Errorf("会话 %s 要用工具 %s，但应用没登记它", scopeID, name)
		}
		table[name] = tool
	}
	r.perScope[scopeID] = scopeTools{tools: table, only: true}
	return nil
}

// CheckAllowed 检查一份模式的工具名单是否都已经安装，不改变登记处。
func (r *Registry) CheckAllowed(names []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range names {
		_, exists := r.global[name]
		if !exists {
			return fmt.Errorf("应用没登记工具 %s", name)
		}
	}
	return nil
}

// Names 列出当前已安装的全局工具名。
func (r *Registry) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.global))
	for name := range r.global {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DropScope 扔掉一段会话的整张小表。
func (r *Registry) DropScope(scopeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.perScope, scopeID)
}

// tableFor 取或创建会话小表；空作用域即全局表。
func (r *Registry) tableFor(scopeID string) map[string]Tool {
	if scopeID == "" {
		return r.global
	}
	entry, exists := r.perScope[scopeID]
	if !exists {
		entry = scopeTools{tools: make(map[string]Tool)}
		r.perScope[scopeID] = entry
	}
	return entry.tools
}

func schemasFrom(table map[string]Tool) []chat.ToolSchema {
	schemas := make([]chat.ToolSchema, 0, len(table))
	for _, tool := range table {
		schemas = append(schemas, tool.Schema)
	}
	return schemas
}

func scopeName(scopeID string) string {
	if scopeID == "" {
		return "全局"
	}
	return scopeID
}
