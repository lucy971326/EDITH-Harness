package chat

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var suggestionPrefixPattern = regexp.MustCompile(`^[!/@$]$`)

// 活对象。suggestionRegistry 保存 Chat 输入候选来源。
type suggestionRegistry struct {
	mu      sync.RWMutex
	sources map[string][]suggestionSourceEntry
	ids     map[string]struct{}
}

type suggestionSourceEntry struct {
	id     string
	source SuggestionSource
}

func newSuggestionRegistry() *suggestionRegistry {
	return &suggestionRegistry{
		sources: make(map[string][]suggestionSourceEntry),
		ids:     make(map[string]struct{}),
	}
}

// Register 填入一个输入候选来源。
func (r *suggestionRegistry) Register(source SuggestionSource) error {
	if source == nil {
		return fmt.Errorf("chat: register nil suggestion source")
	}
	id := strings.TrimSpace(source.ID())
	if id == "" {
		return fmt.Errorf("chat: suggestion source ID is empty")
	}
	prefixes := source.Prefixes()
	if len(prefixes) == 0 {
		return fmt.Errorf("chat: suggestion source %q has no prefix", id)
	}
	normalized := make([]string, 0, len(prefixes))
	seenPrefixes := make(map[string]struct{}, len(prefixes))
	for _, rawPrefix := range prefixes {
		prefix := strings.TrimSpace(rawPrefix)
		if !suggestionPrefixPattern.MatchString(prefix) {
			return fmt.Errorf("chat: suggestion prefix %q is invalid", prefix)
		}
		if _, duplicate := seenPrefixes[prefix]; duplicate {
			return fmt.Errorf("chat: suggestion source %q repeats prefix %q", id, prefix)
		}
		seenPrefixes[prefix] = struct{}{}
		normalized = append(normalized, prefix)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.ids[id]; exists {
		return fmt.Errorf("chat: suggestion source %q already registered", id)
	}
	for _, prefix := range normalized {
		r.sources[prefix] = append(r.sources[prefix], suggestionSourceEntry{id: id, source: source})
	}
	r.ids[id] = struct{}{}
	return nil
}

// List 查询一个前缀下的候选并按来源 ID、名称稳定合并。
func (r *suggestionRegistry) List(prefix string, context SuggestionContext) ([]Suggestion, error) {
	prefix = strings.TrimSpace(prefix)
	r.mu.RLock()
	sources := append([]suggestionSourceEntry(nil), r.sources[prefix]...)
	r.mu.RUnlock()
	if len(sources) == 0 {
		return nil, nil
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].id < sources[j].id
	})
	all := make([]Suggestion, 0)
	for _, entry := range sources {
		items, err := entry.source.List(context)
		if err != nil {
			return nil, fmt.Errorf("chat: list suggestions for %q from %q: %w", prefix, entry.id, err)
		}
		items = append([]Suggestion(nil), items...)
		seen := make(map[string]struct{}, len(items))
		for i := range items {
			name := strings.TrimSpace(items[i].Name)
			if name == "" {
				return nil, fmt.Errorf("chat: suggestion source %q returned empty name", entry.id)
			}
			if _, duplicate := seen[name]; duplicate {
				return nil, fmt.Errorf("chat: suggestion source %q returned duplicate %q", entry.id, name)
			}
			items[i].Name = name
			items[i].SourceID = entry.id
			seen[name] = struct{}{}
		}
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].Name < items[j].Name
		})
		all = append(all, items...)
	}
	return all, nil
}
