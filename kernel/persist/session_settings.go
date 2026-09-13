package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"harness/kernel/session/settings"
)

func (s *jsonl) For(sessionID string) (settings.SessionSettings, error) {
	err := checkID(sessionID)
	if err != nil {
		return settings.SessionSettings{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := s.sessionFiles(sessionID)
	if err != nil {
		return settings.SessionSettings{}, err
	}
	b, err := files.Read("settings.json")
	if err != nil {
		return settings.SessionSettings{}, err
	}

	var out settings.SessionSettings
	err = json.Unmarshal(b, &out)
	if err != nil {
		return settings.SessionSettings{}, fmt.Errorf("persist: session settings %q: %w", sessionID, err)
	}
	return out, nil
}

func (s *jsonl) Put(sessionID string, in settings.SessionSettings) error {
	err := checkID(sessionID)
	if err != nil {
		return err
	}

	b, err := json.Marshal(in)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := s.sessionFiles(sessionID)
	if err != nil {
		return err
	}
	return files.Write("settings.json", b)
}

// UsesAgent 返回是否仍有会话选择了指定 Agent。
func (s *jsonl) UsesAgent(agentID string) (bool, error) {
	if err := checkID(agentID); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, err := s.files.Scope("sessions")
	if err != nil {
		return false, err
	}
	entries, err := sessions.List()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir {
			continue
		}
		files, err := sessions.Scope(entry.Name)
		if err != nil {
			return false, err
		}
		data, err := files.Read("settings.json")
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		var sessionSettings settings.SessionSettings
		if err := json.Unmarshal(data, &sessionSettings); err != nil {
			return false, fmt.Errorf("persist: session settings %q: %w", entry.Name, err)
		}
		if sessionSettings.AgentID == agentID {
			return true, nil
		}
	}
	return false, nil
}
