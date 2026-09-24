package approvals

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"harness/internal/permissions"
	"harness/internal/persist"

	"gopkg.in/yaml.v3"
)

// ErrSettings 表示用户提交的审核设置不可用。
var ErrSettings = errors.New("approvals: invalid settings")

func (s *Service) loadSettings(files *persist.Files) error {
	// 与 llm 读取同一个文件，各自只解析自己的配置项。
	body, err := files.Read("config.yaml")
	if err != nil {
		return err
	}
	var config struct {
		Jev struct {
			APIKey string `yaml:"apiKey"`
		} `yaml:"jev"`
	}
	err = yaml.Unmarshal(body, &config)
	if err != nil {
		return fmt.Errorf("approvals: invalid config.yaml")
	}
	s.jevKey = strings.TrimSpace(config.Jev.APIKey)
	s.files, err = files.Scope("approvals")
	if err != nil {
		return err
	}
	body, err = s.files.Read("settings.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, &s.settings)
	if err != nil {
		return fmt.Errorf("approvals: decode settings: %w", err)
	}
	if s.settings.Engine != "llm" && s.settings.Engine != "jev" {
		return fmt.Errorf("approvals: unknown saved engine")
	}
	// Provider 被移除时保留选择，界面置为不可用，实际申请转人工。
	return nil
}

// Settings 返回后端设置及可用性，不暴露密钥。
func (s *Service) Settings() SettingsView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SettingsView{Settings: s.settings, JevConfigured: s.jevKey != "", Available: s.validateSettings(s.settings) == nil}
}

// SaveSettings 写盘成功后再替换内存设置；正在审核的快照不变。
func (s *Service) SaveSettings(next Settings) (SettingsView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return SettingsView{}, fmt.Errorf("approvals: service closed")
	}
	err := s.validateSettings(next)
	if err != nil {
		return SettingsView{}, fmt.Errorf("%w: %v", ErrSettings, err)
	}
	if s.files == nil {
		return SettingsView{}, fmt.Errorf("approvals: settings storage unavailable")
	}
	body, err := json.Marshal(next)
	if err != nil {
		return SettingsView{}, err
	}
	err = s.files.Write("settings.json", body)
	if err != nil {
		return SettingsView{}, err
	}
	s.settings = next
	return SettingsView{Settings: next, JevConfigured: s.jevKey != "", Available: true}, nil
}

func (s *Service) validateSettings(value Settings) error {
	switch value.Engine {
	case "jev":
		if s.jevKey == "" {
			return fmt.Errorf("请在 config.yaml 配置 jev.apiKey 并重启")
		}
	case "llm":
		if s.models != nil {
			for _, model := range s.models.Models() {
				if model.ID == value.Model && slices.Contains(model.ReasoningEfforts, value.ReasoningEffort) {
					return s.models.ValidateConfig(value.Model)
				}
			}
		}
		return fmt.Errorf("请选择可用模型与思考档位")
	default:
		return fmt.Errorf("未知审核方式")
	}
	return nil
}

// Modes 在纯规则目录上补充当前部署的智能审批可用性。
func (s *Service) Modes() []permissions.ModeChoice {
	modes := permissions.Modes()
	available := s.Settings().Available
	for i := range modes {
		if modes[i].ID == permissions.ApproveForMe {
			modes[i].Available = available
		}
	}
	return modes
}
