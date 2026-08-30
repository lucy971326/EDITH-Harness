package prompts

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"harness/kernel/kinds"
)

// 活对象。挂在 Host 的 prompts 键上的提示词登记处。
type Registry struct {
	mu    sync.RWMutex
	parts []Part
}

// NewRegistry 造一张空的提示词登记处。
func NewRegistry() *Registry {
	return &Registry{}
}

// Register 填入一段公共提示词，并返回幂等注销函数。
func (r *Registry) Register(part Part) (func(), error) {
	err := validatePart(part)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, registered := range r.parts {
		if registered.Name == part.Name {
			return nil, fmt.Errorf("prompts: %q already registered", part.Name)
		}
	}
	r.parts = append(r.parts, part)

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			for i, registered := range r.parts {
				if registered.Name == part.Name {
					r.parts = append(r.parts[:i], r.parts[i+1:]...)
					return
				}
			}
		})
	}, nil
}

// Assemble 合并公共提示词和 Agent 本轮提示词，按 Order 稳定排序后拼接。
func (r *Registry) Assemble(ctx context.Context, setup kinds.Setup, local ...Part) (string, error) {
	err := ctx.Err()
	if err != nil {
		return "", err
	}

	r.mu.RLock()
	parts := make([]Part, len(r.parts), len(r.parts)+len(local))
	copy(parts, r.parts)
	r.mu.RUnlock()

	names := make(map[string]struct{}, len(parts)+len(local))
	for _, part := range parts {
		names[part.Name] = struct{}{}
	}
	for _, part := range local {
		err = validatePart(part)
		if err != nil {
			return "", err
		}
		if _, exists := names[part.Name]; exists {
			return "", fmt.Errorf("prompts: duplicate part %q", part.Name)
		}
		names[part.Name] = struct{}{}
		parts = append(parts, part)
	}

	sort.SliceStable(parts, func(i, j int) bool {
		return parts[i].Order < parts[j].Order
	})

	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		err = ctx.Err()
		if err != nil {
			return "", err
		}

		value, renderErr := part.Render(ctx, setup)
		if err = ctx.Err(); err != nil {
			return "", err
		}
		if renderErr != nil {
			return "", fmt.Errorf("prompts: render %q: %w", part.Name, renderErr)
		}

		value = strings.TrimSpace(value)
		if value != "" {
			texts = append(texts, value)
		}
	}
	return strings.Join(texts, "\n\n"), nil
}

func validatePart(part Part) error {
	if part.Name == "" {
		return fmt.Errorf("prompts: part has empty name")
	}
	if part.Render == nil {
		return fmt.Errorf("prompts: part %q has no render", part.Name)
	}
	return nil
}
