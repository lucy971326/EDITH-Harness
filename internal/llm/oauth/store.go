package oauth

import (
	"errors"
	"fmt"

	"harness/internal/persist"
)

// ErrUnreadable 表示文件存在，但当前用户无法解开凭据内容。
var ErrUnreadable = errors.New("model OAuth credential cannot be decoded")

// 活对象。一家模型服务的本机私有凭据文件。
type Store struct {
	files *persist.Files
	name  string
}

// NewStore 为一家服务选择固定的 model-auth 凭据文件。
func NewStore(files *persist.Files, name string) (*Store, error) {
	scope, err := files.Scope("model-auth")
	if err != nil {
		return nil, err
	}
	return &Store{files: scope, name: name + ".json"}, nil
}

func (s *Store) Read() ([]byte, error) {
	sealed, err := s.files.Read(s.name)
	if err != nil {
		return nil, err
	}
	plain, err := openCredential(sealed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	return plain, nil
}

func (s *Store) Write(data []byte) error {
	sealed, err := sealCredential(data)
	if err != nil {
		return err
	}
	return s.files.Write(s.name, sealed)
}

func (s *Store) Remove() error { return s.files.Remove(s.name) }
