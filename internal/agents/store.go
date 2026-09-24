package agents

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"harness/internal/persist"
)

// 活对象。Agent 设置的文件存储。
type fileStore struct {
	files *persist.Files
}

// NewStore 创建 Agent 设置存储。
func NewStore(files *persist.Files) AgentStore {
	return &fileStore{files: files}
}

func (s *fileStore) agentFiles() (*persist.Files, error) {
	return s.files.Scope("agents")
}

func (s *fileStore) ListAgents() ([]Agent, error) {
	files, err := s.agentFiles()
	if err != nil {
		return nil, err
	}
	entries, err := files.List()
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir || !strings.HasSuffix(entry.Name, ".json") {
			continue
		}
		b, err := files.Read(entry.Name)
		if err != nil {
			return nil, err
		}
		var agent Agent
		if err := json.Unmarshal(b, &agent); err != nil {
			return nil, fmt.Errorf("agents: agent %q: %w", entry.Name, err)
		}
		out = append(out, agent)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *fileStore) ForAgent(id string) (Agent, error) {
	if err := checkID(id); err != nil {
		return Agent{}, err
	}

	files, err := s.agentFiles()
	if err != nil {
		return Agent{}, err
	}
	b, err := files.Read(id + ".json")
	if err != nil {
		return Agent{}, err
	}
	var agent Agent
	if err := json.Unmarshal(b, &agent); err != nil {
		return Agent{}, fmt.Errorf("agents: agent %q: %w", id, err)
	}
	return agent, nil
}

func (s *fileStore) PutAgent(agent Agent) error {
	if err := checkID(agent.ID); err != nil {
		return err
	}
	b, err := json.Marshal(agent)
	if err != nil {
		return err
	}

	files, err := s.agentFiles()
	if err != nil {
		return err
	}
	return files.Write(agent.ID+".json", b)
}

func (s *fileStore) DeleteAgent(id string) error {
	if err := checkID(id); err != nil {
		return err
	}

	files, err := s.agentFiles()
	if err != nil {
		return err
	}
	return files.Remove(id + ".json")
}

func checkID(id string) error {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("agents: bad id %q", id)
	}
	return nil
}
