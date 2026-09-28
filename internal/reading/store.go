// Package reading 保存当前本机用户的会话阅读位置，不拥有对话或运行事实。
package reading

import (
	"encoding/json"
	"errors"
	"fmt"
	"harness/internal/persist"
	"os"
	"sync"
)

// 活对象。通过共享锁保证多连接的阅读位置只能前进；磁盘是唯一事实来源。
type Store struct {
	mu    sync.Mutex
	files *persist.Files
}

// New 组装阅读位置存储，不启动后台资源。
func New(files *persist.Files) (*Store, error) {
	if files == nil {
		return nil, fmt.Errorf("reading: nil files")
	}
	dir, err := files.Scope("reading")
	if err != nil {
		return nil, err
	}
	return &Store{files: dir}, nil
}

// Position 返回最后已读结果对应的 Run 起始账本序号，缺失时为零。
func (s *Store) Position(sessionID string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.position(sessionID)
}

func (s *Store) position(sessionID string) (uint64, error) {
	data, err := s.files.Read(sessionID + ".json")
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var position uint64
	err = json.Unmarshal(data, &position)
	if err != nil {
		return 0, fmt.Errorf("reading: invalid position: %w", err)
	}
	return position, nil
}

// Advance 原子前进；迟到的旧客户端不能把较新的阅读位置覆盖掉。
func (s *Store) Advance(sessionID string, position uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.position(sessionID)
	if err != nil {
		return err
	}
	if position <= old {
		return nil
	}
	data, err := json.Marshal(position)
	if err != nil {
		return err
	}
	return s.files.Write(sessionID+".json", data)
}
