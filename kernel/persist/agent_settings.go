package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"harness/kernel/agents/config"
)

func (s *jsonl) agentFile(id string) string {
	return filepath.Join(s.dir, id+".agent.json")
}

func (s *jsonl) ListAgents() ([]config.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	paths, err := filepath.Glob(filepath.Join(s.dir, "*.agent.json"))
	if err != nil {
		return nil, err
	}
	out := make([]config.Agent, 0, len(paths))
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var agent config.Agent
		if err := json.Unmarshal(b, &agent); err != nil {
			return nil, fmt.Errorf("persist: agent %q: %w", filepath.Base(path), err)
		}
		out = append(out, agent)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *jsonl) ForAgent(id string) (config.Agent, error) {
	if err := checkID(id); err != nil {
		return config.Agent{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.agentFile(id))
	if err != nil {
		return config.Agent{}, err
	}
	var agent config.Agent
	if err := json.Unmarshal(b, &agent); err != nil {
		return config.Agent{}, fmt.Errorf("persist: agent %q: %w", id, err)
	}
	return agent, nil
}

func (s *jsonl) PutAgent(agent config.Agent) error {
	if err := checkID(agent.ID); err != nil {
		return err
	}
	b, err := json.Marshal(agent)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.agentFile(agent.ID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

func (s *jsonl) DeleteAgent(id string) error {
	if err := checkID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(s.agentFile(id))
}
