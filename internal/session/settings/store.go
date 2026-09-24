package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"harness/internal/permissions"
	"harness/internal/persist"
)

// 活对象。会话运行设置的文件存储；Agent 引用事务由 agents.Service 协调。
type Store struct {
	files *persist.Files
}

// NewStore 创建会话设置存储。
func NewStore(files *persist.Files) *Store {
	return &Store{files: files}
}

func (s *Store) For(sessionID string) (SessionSettings, error) {
	files, err := s.sessionFiles(sessionID)
	if err != nil {
		return SessionSettings{}, err
	}
	b, err := files.Read("settings.json")
	if err != nil {
		return SessionSettings{}, err
	}

	var out SessionSettings
	err = json.Unmarshal(b, &out)
	if err != nil {
		return SessionSettings{}, fmt.Errorf("settings: session %q: %w", sessionID, err)
	}
	if out.PermissionMode == "" {
		return SessionSettings{}, fmt.Errorf("settings: missing permission mode")
	}
	out.PermissionMode, err = permissions.NormalizeMode(out.PermissionMode)
	if err != nil {
		return SessionSettings{}, fmt.Errorf("settings: session %q: %w", sessionID, err)
	}
	return out, nil
}

func (s *Store) Put(sessionID string, in SessionSettings) error {
	mode, err := permissions.NormalizeMode(in.PermissionMode)
	if err != nil {
		return fmt.Errorf("settings: session %q: %w", sessionID, err)
	}
	in.PermissionMode = mode
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}

	files, err := s.sessionFiles(sessionID)
	if err != nil {
		return err
	}
	return files.Write("settings.json", b)
}

// UsesAgent 返回是否仍有会话选择了指定 Agent。
func (s *Store) UsesAgent(agentID string) (bool, error) {
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
		var sessionSettings SessionSettings
		if err := json.Unmarshal(data, &sessionSettings); err != nil {
			return false, fmt.Errorf("settings: session %q: %w", entry.Name, err)
		}
		if sessionSettings.AgentID == agentID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) sessionFiles(id string) (*persist.Files, error) {
	return s.files.Scope("sessions", id)
}
