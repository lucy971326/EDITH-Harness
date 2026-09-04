package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"harness/kernel/session/settings"
)

func (s *jsonl) sessionSettingsFile(id string) string {
	return filepath.Join(s.sessionDir(id), "settings.json")
}

func (s *jsonl) For(sessionID string) (settings.SessionSettings, error) {
	err := checkID(sessionID)
	if err != nil {
		return settings.SessionSettings{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.sessionSettingsFile(sessionID))
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
	err = s.ensureSessionDir(sessionID)
	if err != nil {
		return err
	}

	path := s.sessionSettingsFile(sessionID)
	tmp := path + ".tmp"
	err = os.WriteFile(tmp, b, 0o644)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// UsesAgent 返回是否仍有会话选择了指定 Agent。
func (s *jsonl) UsesAgent(agentID string) (bool, error) {
	if err := checkID(agentID); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(filepath.Join(s.dir, "sessions"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(s.dir, "sessions", entry.Name(), "settings.json")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		var sessionSettings settings.SessionSettings
		if err := json.Unmarshal(data, &sessionSettings); err != nil {
			return false, fmt.Errorf("persist: session settings %q: %w", entry.Name(), err)
		}
		if sessionSettings.AgentID == agentID {
			return true, nil
		}
	}
	return false, nil
}
