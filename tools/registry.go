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
	perAgent map[string]map[string]Tool
}

// NewRegistry 建一个空的登记处。
func NewRegistry() *Registry {
	return &Registry{
		global:   make(map[string]Tool),
		perAgent: make(map[string]map[string]Tool),
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
		tool, ok := r.perAgent[agent][name]
		if ok {
			return tool, true
		}
	}
	tool, ok := r.global[name]
	return tool, ok
}

// Schemas 列出 agent 能看见的全部工具说明书（先全局、再本 agent 遮蔽版），给模型看。
func (r *Registry) Schemas(agent string) []chat.ToolSchema {
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := make(map[string]bool)
	var schemas []chat.ToolSchema
	for name, tool := range r.global {
		schemas = append(schemas, tool.Schema)
		seen[name] = true
	}
	if agent != "" {
		for name, tool := range r.perAgent[agent] {
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
	return schemas
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
	table, exists := r.perAgent[agent]
	if !exists {
		table = make(map[string]Tool)
		r.perAgent[agent] = table
	}
	return table
}

func scopeName(agent string) string {
	if agent == "" {
		return "全局"
	}
	return agent
}
