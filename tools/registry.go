package tools

import (
	"fmt"
	"sync"

	"harness/chat"
)

// Registry 是工具登记处：一张全局表 + 每个 agent 一张小表（同名遮蔽）。
// 查找先小表后大表；agent 死了，DropAgent 把它的小表整张扔掉。
type Registry struct {
	mu       sync.Mutex
	global   map[string]Tool
	perAgent map[string]agentTools
}

// agentTools 是一个 Agent 的工具小表；only=true 时名单以外的全局工具也不可见。
type agentTools struct {
	tools map[string]Tool
	only  bool
}

// NewRegistry 建一个空的登记处。
func NewRegistry() *Registry {
	return &Registry{
		global:   make(map[string]Tool),
		perAgent: make(map[string]agentTools),
	}
}

// Register 登记一个全局工具；同名重复登记是组装错误。
func (r *Registry) Register(tool Tool) error {
	return r.register("", tool)
}

// RegisterFor 给某个 agent 登记专用工具，遮蔽同名全局工具（沙箱版替换普通版）。
func (r *Registry) RegisterFor(agent string, tool Tool) error {
	if agent == "" {
		return fmt.Errorf("专用工具要写明是哪个 agent 的")
	}
	return r.register(agent, tool)
}

func (r *Registry) register(agent string, tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	table := r.tableFor(agent)
	name := tool.Schema.Name
	_, taken := table[name]
	if taken {
		return fmt.Errorf("工具 %s 在 %s 的表里已登记过", name, scopeName(agent))
	}
	table[name] = tool
	return nil
}

// Lookup 查工具：agent 的小表命中即遮蔽全局，查不到落大表。
func (r *Registry) Lookup(name string, agent string) (Tool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if agent != "" {
		table, exists := r.perAgent[agent]
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

// Schemas 列出 agent 能看见的全部工具说明书（先全局、再本 agent 遮蔽版），给模型看。
func (r *Registry) Schemas(agent string) []chat.ToolSchema {
	r.mu.Lock()
	defer r.mu.Unlock()

	if agent != "" {
		table, exists := r.perAgent[agent]
		if exists && table.only {
			return schemasFrom(table.tools)
		}
	}
	seen := make(map[string]bool)
	schemas := schemasFrom(r.global)
	for _, schema := range schemas {
		seen[schema.Name] = true
	}
	if agent != "" {
		table, exists := r.perAgent[agent]
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

// SetAllowed 给一个 Agent 绑定明确的工具白名单。名单外的全局工具也不可见；
// 名单里的每个工具必须已由应用启动时登记。
func (r *Registry) SetAllowed(agent string, names []string) error {
	if agent == "" {
		return fmt.Errorf("Agent 工具白名单必须写明 agent")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	table := make(map[string]Tool, len(names))
	for _, name := range names {
		tool, exists := r.global[name]
		if !exists {
			return fmt.Errorf("agent %s 要用工具 %s，但应用没登记它", agent, name)
		}
		table[name] = tool
	}
	r.perAgent[agent] = agentTools{tools: table, only: true}
	return nil
}

// CheckAllowed 检查一个 Agent 档案的工具名单是否都已经安装，不改变登记处。
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

// Names 列出当前已安装的全局工具名；主要给组装层生成 Agent 档案用。
func (r *Registry) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.global))
	for name := range r.global {
		names = append(names, name)
	}
	return names
}

// DropAgent 扔掉某个 agent 的整张小表；agent 关闭时收摊用，防泄漏。
func (r *Registry) DropAgent(agent string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.perAgent, agent)
}

// tableFor 取（或建）agent 的表；空 agent 名即全局表。
func (r *Registry) tableFor(agent string) map[string]Tool {
	if agent == "" {
		return r.global
	}
	entry, exists := r.perAgent[agent]
	if !exists {
		entry = agentTools{tools: make(map[string]Tool)}
		r.perAgent[agent] = entry
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

func scopeName(agent string) string {
	if agent == "" {
		return "全局"
	}
	return agent
}
