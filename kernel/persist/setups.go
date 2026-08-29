package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"harness/kernel/kinds"
)

func (s *jsonl) setupFile(id string) string {
	return filepath.Join(s.dir, id+".setup.json")
}

func (s *jsonl) For(sessionID string) (kinds.Setup, error) {
	err := checkID(sessionID)
	if err != nil {
		return kinds.Setup{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.setupFile(sessionID))
	if err != nil {
		return kinds.Setup{}, err
	}

	var out kinds.Setup
	err = json.Unmarshal(b, &out)
	if err != nil {
		return kinds.Setup{}, fmt.Errorf("persist: setup %q: %w", sessionID, err)
	}
	return cloneSetup(out), nil
}

func (s *jsonl) Put(sessionID string, in kinds.Setup) error {
	err := checkID(sessionID)
	if err != nil {
		return err
	}

	stored := cloneSetup(in)
	b, err := json.Marshal(stored)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	err = os.WriteFile(s.setupFile(sessionID), b, 0o644)
	if err != nil {
		return err
	}
	return nil
}

func cloneSetup(s kinds.Setup) kinds.Setup {
	s.Tools = append([]string(nil), s.Tools...)
	return s
}
