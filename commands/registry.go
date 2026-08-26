package commands

import (
	"fmt"
	"sort"
	"sync"
)

var _ Service = (*Registry)(nil)

// Registry 是命令登记处：一张全局表，登记、查找、撤销。
type Registry struct {
	mu       sync.Mutex
	commands map[string]*Command
}

// NewRegistry 建一个空登记处。
func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]*Command)}
}

// Register 登记一条命令；同名重复是组装错误。
// 返回的 unregister 幂等：只撤销这一次登记。
func (r *Registry) Register(command Command) (func(), error) {
	normalized, err := normalizeCommand(command)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, taken := r.commands[normalized.Name]
	if taken {
		return nil, fmt.Errorf("命令 %s 已登记", normalized.Name)
	}
	entry := &Command{}
	*entry = normalized
	r.commands[normalized.Name] = entry
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		current, exists := r.commands[normalized.Name]
		if !exists {
			return
		}
		if current != entry {
			return
		}
		delete(r.commands, normalized.Name)
	}, nil
}

// List 按分组再按名字列出当前说明书。
func (r *Registry) List() []Descriptor {
	r.mu.Lock()
	defer r.mu.Unlock()
	listed := make([]Descriptor, 0, len(r.commands))
	for _, command := range r.commands {
		listed = append(listed, descriptorOf(*command))
	}
	sort.Slice(listed, func(i int, j int) bool {
		if listed[i].Group != listed[j].Group {
			return listed[i].Group < listed[j].Group
		}
		return listed[i].Name < listed[j].Name
	})
	return listed
}

// Find 按名字取一条命令；没有则第二返回值为 false。
func (r *Registry) Find(name string) (Command, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, exists := r.commands[name]
	if !exists {
		return Command{}, false
	}
	return *entry, true
}

func descriptorOf(command Command) Descriptor {
	return Descriptor{
		Name:        command.Name,
		Description: command.Description,
		Group:       command.Group,
		Hint:        command.Hint,
	}
}
