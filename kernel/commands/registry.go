package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// 活对象。挂在 Host 的 commands 键上的命令登记处。
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Command
}

// NewRegistry 造一张空的命令登记处。
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]Command)}
}

// Register 填入一条启动时固定的命令。
func (r *Registry) Register(command Command) error {
	if command == nil {
		return fmt.Errorf("commands: register nil command")
	}
	name := strings.TrimSpace(command.Name())
	if name == "" {
		return fmt.Errorf("commands: register empty name")
	}
	if strings.TrimSpace(command.Description()) == "" {
		return fmt.Errorf("commands: register %q with empty description", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[name]; exists {
		return fmt.Errorf("commands: %q already registered", name)
	}
	r.entries[name] = command
	return nil
}

// Get 按名取一条已登记命令。
func (r *Registry) Get(name string) (Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	command, ok := r.entries[strings.TrimSpace(name)]
	if !ok {
		return nil, fmt.Errorf("commands: %q is not registered", name)
	}
	return command, nil
}

// List 按名称稳定列出当前命令。
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.entries))
	for _, command := range r.entries {
		out = append(out, Definition{Name: command.Name(), Description: command.Description()})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// Call 按名执行一条命令。
func (r *Registry) Call(ctx context.Context, name, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("commands: empty session id")
	}
	command, err := r.Get(name)
	if err != nil {
		return err
	}
	return command.Run(ctx, sessionID)
}
