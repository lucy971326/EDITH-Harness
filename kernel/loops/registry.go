package loops

import (
	"fmt"
	"sort"
	"sync"
)

// 活对象。挂在 Host 的 loops 键上的运行范式登记处。
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Loop
}

// NewRegistry 造一张空的运行范式登记处。
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]Loop)}
}

// Register 填入一种运行范式，并返回幂等注销函数。
func (r *Registry) Register(loop Loop) (func(), error) {
	if loop == nil {
		return nil, fmt.Errorf("loops: register nil loop")
	}
	definition := loop.Definition()
	if definition.Kind == "" {
		return nil, fmt.Errorf("loops: register empty kind")
	}
	if definition.Description == "" {
		return nil, fmt.Errorf("loops: register %q with empty description", definition.Kind)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[definition.Kind]; exists {
		return nil, fmt.Errorf("loops: %q already registered", definition.Kind)
	}
	r.entries[definition.Kind] = loop

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			delete(r.entries, definition.Kind)
		})
	}, nil
}

// Get 按 Kind 取一种已登记的运行范式。
func (r *Registry) Get(kind string) (Loop, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	loop, ok := r.entries[kind]
	if !ok {
		return nil, fmt.Errorf("loops: %q is not registered", kind)
	}
	return loop, nil
}

// Definitions 按 Kind 稳定列出当前可选的运行范式。
func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.entries))
	for _, loop := range r.entries {
		out = append(out, loop.Definition())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Kind < out[j].Kind
	})
	return out
}
