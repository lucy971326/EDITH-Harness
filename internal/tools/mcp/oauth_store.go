package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"harness/internal/persist"
	"os"
	"strings"
	"sync"

	"golang.org/x/oauth2"
)

var ErrOAuthCredentialCleanup = errors.New("mcp: OAuth credential cleanup failed")

// 数据。令牌和客户端身份保存在 MCP 私有文件中，不进入公开配置。
type oauthRecord struct {
	Resource     string           `json:"resource"`
	Issuer       string           `json:"issuer"`
	ClientID     string           `json:"clientId"`
	ClientSecret string           `json:"clientSecret,omitempty"`
	AuthURL      string           `json:"authUrl"`
	TokenURL     string           `json:"tokenUrl"`
	AuthStyle    oauth2.AuthStyle `json:"authStyle"`
	Scopes       []string         `json:"scopes"`
	Token        oauth2.Token     `json:"token"`
}

// 活对象。串行化凭据文件写入，并标记退出登录后的连接代数。
type oauthStore struct {
	mu         sync.Mutex
	files      *persist.Files
	memory     map[string]oauthRecord
	generation map[string]uint64
	epoch      map[string]uint64
	revoked    map[string]bool
	refreshMu  map[string]*sync.Mutex
	required   map[string][]string
	memoryOnly bool // 测试用；避免访问宿主用户数据目录。
}

func newOAuthStore() *oauthStore {
	return &oauthStore{memory: make(map[string]oauthRecord), generation: make(map[string]uint64), epoch: make(map[string]uint64), revoked: make(map[string]bool), refreshMu: make(map[string]*sync.Mutex), required: make(map[string][]string)}
}

func oauthKey(workspace string, project bool, spec serverSpec) string {
	scope := "global"
	if project {
		scope = "project:" + workspace
	}
	configuredURL := spec.ConfigURL
	if configuredURL == "" {
		configuredURL = spec.URL
	}
	hash := sha256.Sum256([]byte(strings.Join([]string{scope, spec.Name, configuredURL,
		spec.OAuthClientID, spec.OAuthClientIDMetadataURL, spec.OAuthClientSecretEnv, spec.OAuthRedirectURL}, "\x00")))
	return hex.EncodeToString(hash[:])
}

func (s *oauthStore) read(key string) (oauthRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.memory[key]; ok {
		return record, true, nil
	}
	if s.revoked[key] {
		return oauthRecord{}, false, nil
	}
	if s.memoryOnly || s.files == nil {
		return oauthRecord{}, false, nil
	}
	data, err := s.files.Read(key + ".cred")
	if errors.Is(err, os.ErrNotExist) {
		return oauthRecord{}, false, nil
	}
	if err != nil {
		return oauthRecord{}, false, fmt.Errorf("mcp: read OAuth credential: %w", err)
	}
	data, err = openOAuthCredential(data)
	if err != nil {
		return oauthRecord{}, false, fmt.Errorf("mcp: open OAuth credential: %w", err)
	}
	var record oauthRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return oauthRecord{}, false, fmt.Errorf("mcp: invalid saved OAuth credential: %w", err)
	}
	return record, true, nil
}

func (s *oauthStore) write(key string, record oauthRecord) (bool, error) {
	return s.writeVersion(key, s.version(key), record)
}

func (s *oauthStore) writeVersion(key string, version uint64, record oauthRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation[key] != version {
		return false, errors.New("OAuth 已退出登录")
	}
	persisted, err := s.save(key, record)
	if persisted {
		delete(s.required, key)
	}
	return persisted, err
}

// save 在 s.mu 下先持久化再更新内存状态；刷新失败时由调用方保留新令牌。
func (s *oauthStore) save(key string, record oauthRecord) (bool, error) {
	if s.memoryOnly || s.files == nil {
		s.memory[key] = record
		delete(s.revoked, key)
		return false, nil
	}
	data, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	data, err = sealOAuthCredential(data)
	if err != nil {
		return false, fmt.Errorf("mcp: protect OAuth credential: %w", err)
	}
	if err := s.files.Write(key+".cred", data); err != nil {
		return false, fmt.Errorf("mcp: save OAuth credential: %w", err)
	}
	delete(s.memory, key)
	delete(s.revoked, key)
	return true, nil
}

func (s *oauthStore) issue(key string, version uint64, record oauthRecord) (bool, uint64, error) {
	return s.issueAtEpoch(key, version, 0, false, record)
}

func (s *oauthStore) restoreIfEpoch(key string, version, epoch uint64, record oauthRecord) error {
	_, _, err := s.issueAtEpoch(key, version, epoch, true, record)
	return err
}

func (s *oauthStore) issueAtEpoch(key string, version, expectedEpoch uint64, conditional bool, record oauthRecord) (bool, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation[key] != version {
		return false, 0, errors.New("OAuth 已退出登录")
	}
	if conditional && s.epoch[key] != expectedEpoch {
		return false, 0, nil
	}
	persisted, err := s.save(key, record)
	if err != nil {
		return false, 0, err
	}
	s.epoch[key]++
	delete(s.required, key)
	return persisted, s.epoch[key], nil
}

func (s *oauthStore) refresh(key string, version, epoch uint64, record oauthRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation[key] != version {
		return errors.New("OAuth 已退出登录")
	}
	if s.epoch[key] != epoch {
		return nil
	} // 旧 Run 可继续使用自己的令牌，但不能覆盖新登录。
	_, err := s.save(key, record)
	if err != nil {
		// 服务端可能已经轮换 refresh token；进程内继续保留新值供重试。
		s.memory[key] = record
		return err
	}
	return nil
}

func (s *oauthStore) requireScopes(key string, scopes []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, scope := range scopes {
		found := false
		for _, old := range s.required[key] {
			if old == scope {
				found = true
				break
			}
		}
		if !found {
			s.required[key] = append(s.required[key], scope)
		}
	}
}

func (s *oauthStore) scopes(key string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.required[key]...)
}

func (s *oauthStore) delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.files != nil && !s.memoryOnly {
		err := s.files.Remove(key + ".cred")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("mcp: remove OAuth credential: %w", err)
		}
	}
	delete(s.memory, key)
	delete(s.refreshMu, key)
	s.revoked[key] = true
	delete(s.required, key)
	s.generation[key]++
	return nil
}

// retire 清掉持久凭据，旧 Run 仍可用自己的连接与令牌完成；刷新不得回写已撤下的配置。
func (s *oauthStore) retire(key string) error {
	return s.retireAtEpoch(key, 0, false)
}

func (s *oauthStore) retireIfEpoch(key string, epoch uint64) error {
	return s.retireAtEpoch(key, epoch, true)
}

func (s *oauthStore) retireAtEpoch(key string, epoch uint64, conditional bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if conditional && s.epoch[key] != epoch {
		return nil
	}
	if s.files != nil && !s.memoryOnly {
		err := s.files.Remove(key + ".cred")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("mcp: remove OAuth credential: %w", err)
		}
	}
	delete(s.memory, key)
	delete(s.refreshMu, key)
	delete(s.required, key)
	s.revoked[key] = true
	s.epoch[key]++
	return nil
}

func (s *oauthStore) refreshLock(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock := s.refreshMu[key]
	if lock == nil {
		lock = &sync.Mutex{}
		s.refreshMu[key] = lock
	}
	return lock
}

func (s *oauthStore) version(key string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation[key]
}

func (s *oauthStore) credentialEpoch(key string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epoch[key]
}
