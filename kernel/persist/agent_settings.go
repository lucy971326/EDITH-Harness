package persist

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"harness/kernel/agents/config"
)

func (s *jsonl) agentFiles() (*Files, error) {
	return s.files.Scope("agents")
}

func (s *jsonl) ListAgents() ([]config.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := s.agentFiles()
	if err != nil {
		return nil, err
	}
	entries, err := files.List()
	if err != nil {
		return nil, err
	}
	out := make([]config.Agent, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir || !strings.HasSuffix(entry.Name, ".json") {
			continue
		}
		b, err := files.Read(entry.Name)
		if err != nil {
			return nil, err
		}
		var agent config.Agent
		if err := json.Unmarshal(b, &agent); err != nil {
			return nil, fmt.Errorf("persist: agent %q: %w", entry.Name, err)
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

	files, err := s.agentFiles()
	if err != nil {
		return config.Agent{}, err
	}
	b, err := files.Read(id + ".json")
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

	files, err := s.agentFiles()
	if err != nil {
		return err
	}
	return files.Write(agent.ID+".json", b)
}

func (s *jsonl) DeleteAgent(id string) error {
	if err := checkID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := s.agentFiles()
	if err != nil {
		return err
	}
	return files.Remove(id + ".json")
}
