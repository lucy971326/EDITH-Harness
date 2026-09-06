package chat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"harness/kernel/agents"
	"harness/kernel/commands"
	"harness/kernel/events"
	"harness/kernel/llm"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/skills"
	"harness/kernel/subagents"
)

// 活对象。聊天业务的统一入口；不拥有账本、Run 或事件登记处。
type Service struct {
	sessions  *session.Store
	settings  settings.SessionSettingsStore
	agents    *agents.Service
	models    *llm.Client
	runner    *runner.Runner
	commands  commands.Commands
	events    *events.Registry
	subagents *subagents.Subagents

	createMu sync.Mutex
}

// NewService 组装聊天业务服务。
func NewService(sessions *session.Store, settingsStore settings.SessionSettingsStore, agentService *agents.Service, modelClient *llm.Client, runService *runner.Runner, commandService commands.Commands, eventRegistry *events.Registry, subagentService *subagents.Subagents) (*Service, error) {
	if sessions == nil {
		return nil, fmt.Errorf("chat service: nil sessions")
	}
	if settingsStore == nil {
		return nil, fmt.Errorf("chat service: nil session settings")
	}
	if agentService == nil {
		return nil, fmt.Errorf("chat service: nil agents")
	}
	if modelClient == nil {
		return nil, fmt.Errorf("chat service: nil llm")
	}
	if runService == nil {
		return nil, fmt.Errorf("chat service: nil runner")
	}
	if commandService == nil {
		return nil, fmt.Errorf("chat service: nil commands")
	}
	if eventRegistry == nil {
		return nil, fmt.Errorf("chat service: nil events")
	}
	if subagentService == nil {
		return nil, fmt.Errorf("chat service: nil subagents")
	}
	return &Service{sessions: sessions, settings: settingsStore, agents: agentService, models: modelClient, runner: runService, commands: commandService, events: eventRegistry, subagents: subagentService}, nil
}

// Create 创建或复用指定工作区中的空会话。
func (s *Service) Create(workspace string) (SessionInfo, error) {
	err := checkWorkspace(workspace)
	if err != nil {
		return SessionInfo{}, err
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()

	infos, err := s.List()
	if err != nil {
		return SessionInfo{}, err
	}
	for _, info := range infos {
		if info.Settings.Workspace != workspace {
			continue
		}
		sess, err := s.sessions.Get(info.Meta.ID)
		if err != nil {
			return SessionInfo{}, fmt.Errorf("chat service: get session: %w", err)
		}
		if len(sess.Entries()) == 0 {
			return info, nil
		}
	}

	id, err := session.NewID()
	if err != nil {
		return SessionInfo{}, err
	}
	_, err = s.sessions.Create(id)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("chat service: create session: %w", err)
	}
	setup := settings.SessionSettings{AgentID: agents.DefaultID, Workspace: workspace}
	err = s.settings.Put(id, setup)
	if err != nil {
		discardErr := s.sessions.DiscardEmpty(id)
		return SessionInfo{}, fmt.Errorf("chat service: save session settings: %w", errors.Join(err, discardErr))
	}
	return s.Session(id)
}

// List 返回全部会话及其运行设置；页面分组和排序由调用方决定。
func (s *Service) List() ([]SessionInfo, error) {
	metas, err := s.sessions.List()
	if err != nil {
		return nil, fmt.Errorf("chat service: list sessions: %w", err)
	}
	out := make([]SessionInfo, 0, len(metas))
	for _, meta := range metas {
		if s.subagents.IsChildSession(meta.ID) {
			continue
		}
		setup, err := s.settings.For(meta.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: settings for %q: %w", ErrSessionSettings, meta.ID, err)
		}
		out = append(out, SessionInfo{Meta: meta, Settings: setup})
	}
	return out, nil
}

// Session 返回一场已存在会话的元数据与运行设置。
func (s *Service) Session(id string) (SessionInfo, error) {
	if s.subagents.IsChildSession(id) {
		return SessionInfo{}, fmt.Errorf("%w: session %q", os.ErrNotExist, id)
	}
	metas, err := s.sessions.List()
	if err != nil {
		return SessionInfo{}, err
	}
	for _, meta := range metas {
		if meta.ID == id {
			setup, err := s.settings.For(id)
			if err != nil {
				return SessionInfo{}, fmt.Errorf("%w: settings for %q: %w", ErrSessionSettings, id, err)
			}
			return SessionInfo{Meta: meta, Settings: setup}, nil
		}
	}
	return SessionInfo{}, fmt.Errorf("%w: session %q", os.ErrNotExist, id)
}

// Snapshot 返回聊天投影所需的耐久账本与活跃 Run。
func (s *Service) Snapshot(sessionID string) (Snapshot, error) {
	sess, err := s.sessions.Get(sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{Entries: sess.Entries(), Runs: []runner.RunState{}}
	if state, ok := s.runner.State(sessionID); ok {
		out.Runs = []runner.RunState{state}
	}
	return out, nil
}

// Start 保存下一轮设置并启动 Runner 自己管理的后台 Run。
func (s *Service) Start(ctx context.Context, input RunInput) error {
	if s.subagents.IsChildSession(input.SessionID) {
		return fmt.Errorf("%w: session %q", os.ErrNotExist, input.SessionID)
	}
	if _, err := s.sessions.Get(input.SessionID); err != nil {
		return err
	}
	err := checkMessage(input.Message)
	if err != nil {
		return err
	}
	setup, err := s.settings.For(input.SessionID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSessionSettings, err)
	}
	err = s.selectRunSettings(&setup, input)
	if err != nil {
		return err
	}
	err = s.ensureVision(setup.Model, input.Message)
	if err != nil {
		return err
	}
	err = s.settings.Put(input.SessionID, setup)
	if err != nil {
		return err
	}
	_, err = s.runner.Start(ctx, input.SessionID, input.Message)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRunStart, err)
	}
	return nil
}

// Steer 将一条输入交给当前 Run；不修改下一轮设置。
func (s *Service) Steer(sessionID string, message session.UserMessage) error {
	if s.subagents.IsChildSession(sessionID) {
		return fmt.Errorf("%w: session %q", os.ErrNotExist, sessionID)
	}
	if _, err := s.sessions.Get(sessionID); err != nil {
		return err
	}
	err := checkMessage(message)
	if err != nil {
		return err
	}
	setup, err := s.settings.For(sessionID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSessionSettings, err)
	}
	err = s.ensureVision(setup.Model, message)
	if err != nil {
		return err
	}
	err = s.runner.Steer(sessionID, message)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRunSteer, err)
	}
	return nil
}

// Stop 取消当前尚未结束的 Run。
func (s *Service) Stop(sessionID string) error { return s.runner.Stop(sessionID) }

// Fork 复制账本中一段已结束助手回答之前的历史与会话设置。
func (s *Service) Fork(input ForkInput) (string, error) {
	if input.SessionID == "" || input.RunID == "" || input.BoundaryEntryID == "" {
		return "", fmt.Errorf("chat service: fork has empty required field")
	}
	if state, running := s.runner.State(input.SessionID); running && state.RunID == input.RunID {
		return "", fmt.Errorf("chat service: assistant response is still running")
	}
	sess, err := s.sessions.Get(input.SessionID)
	if err != nil {
		return "", err
	}
	through, err := assistantSegmentEnd(sess.Entries(), input.RunID, input.BoundaryEntryID)
	if err != nil {
		return "", err
	}
	setup, err := s.settings.For(input.SessionID)
	if err != nil {
		return "", fmt.Errorf("chat service: load session settings: %w", err)
	}
	info, err := s.Session(input.SessionID)
	if err != nil {
		return "", err
	}
	destinationID, err := session.NewID()
	if err != nil {
		return "", err
	}
	err = s.settings.Put(destinationID, setup)
	if err != nil {
		return "", fmt.Errorf("chat service: copy session settings: %w", err)
	}
	_, err = s.sessions.Fork(input.SessionID, destinationID, through, info.Meta.Title+" · 分叉")
	if err != nil {
		return "", fmt.Errorf("chat service: copy session: %w", err)
	}
	return destinationID, nil
}

// SubscribeRun 订阅已有 Runner RunEvent，并返回幂等注销函数。
func (s *Service) SubscribeRun(handler func(context.Context, runner.RunEvent) error) (func(), error) {
	return events.Subscribe(s.events, handler)
}

// Models 返回当前可选模型。
func (s *Service) Models() []llm.ModelChoice { return s.models.Models() }

// AvailableSkills 返回工作区中当前可见的 Skill。
func (s *Service) AvailableSkills(workspace string) ([]skills.Skill, error) {
	return s.agents.AvailableSkills(workspace)
}

// Commands 返回当前平台命令。
func (s *Service) Commands() []commands.Definition { return s.commands.List() }

// CallCommand 执行一条平台命令。
func (s *Service) CallCommand(ctx context.Context, name, sessionID string) error {
	if _, err := s.sessions.Get(sessionID); err != nil {
		return err
	}
	return s.commands.Call(ctx, name, sessionID)
}

// ListAgents 返回全部 Agent 设置。
func (s *Service) ListAgents() ([]agents.Agent, error) { return s.agents.List() }

// GetAgent 返回一个 Agent 设置。
func (s *Service) GetAgent(id string) (agents.Agent, error) { return s.agents.Get(id) }

// SaveAgent 保存一个 Agent 设置。
func (s *Service) SaveAgent(agent agents.Agent) (agents.Agent, error) { return s.agents.Save(agent) }

// DeleteAgent 删除一个未被会话使用的 Agent。
func (s *Service) DeleteAgent(id string) error { return s.agents.Delete(id) }

// AgentInUse 返回指定 Agent 是否仍被会话使用。
func (s *Service) AgentInUse(id string) (bool, error) { return s.agents.InUse(id) }

// AgentChoices 返回可用于 Agent 设置的 Loop 和 Tool。
func (s *Service) AgentChoices() agents.Choices { return s.agents.Choices() }

func (s *Service) selectRunSettings(setup *settings.SessionSettings, input RunInput) error {
	if strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.ReasoningEffort) == "" {
		return fmt.Errorf("请先选择模型和思考档位")
	}
	agentID := strings.TrimSpace(input.AgentID)
	if agentID == "" {
		agentID = setup.AgentID
	}
	if _, err := s.agents.Get(agentID); err != nil {
		return fmt.Errorf("Agent 不可用：%w", err)
	}
	for _, choice := range s.models.Models() {
		if choice.ID != input.Model {
			continue
		}
		for _, effort := range choice.ReasoningEfforts {
			if effort == input.ReasoningEffort {
				setup.AgentID = agentID
				setup.Model = input.Model
				setup.ReasoningEffort = input.ReasoningEffort
				return nil
			}
		}
	}
	return fmt.Errorf("模型或思考档位不可用")
}

func (s *Service) ensureVision(model string, message session.UserMessage) error {
	for _, block := range message.Blocks {
		if block.Kind == "image" && !s.models.Vision(model) {
			return fmt.Errorf("当前模型无法识别图片")
		}
	}
	return nil
}

func checkWorkspace(workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if !filepath.IsAbs(workspace) {
		return fmt.Errorf("工作区目录必须是绝对路径")
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return fmt.Errorf("工作区目录不可用：%w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("工作区路径不是目录")
	}
	return nil
}

func checkMessage(message session.UserMessage) error {
	if len(message.Blocks) == 0 {
		return fmt.Errorf("请输入文字或图片")
	}
	for _, block := range message.Blocks {
		switch block.Kind {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				return nil
			}
		case "image":
			if block.Media != nil && block.Media.MIME != "" && block.Media.Data != "" {
				return nil
			}
		}
	}
	return fmt.Errorf("请输入文字或图片")
}

func assistantSegmentEnd(entries []session.Entry, runID, boundaryEntryID string) (string, error) {
	start := -1
	for index, entry := range entries {
		if entry.ID == boundaryEntryID && entry.Message.RunID == runID && entry.Message.Role == session.RoleUser {
			start = index
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("chat service: assistant segment not found")
	}
	end := ""
	for _, entry := range entries[start+1:] {
		if entry.Message.RunID != runID {
			continue
		}
		if entry.Message.Role == session.RoleUser {
			break
		}
		end = entry.ID
	}
	if end == "" {
		return "", fmt.Errorf("chat service: assistant segment has no durable result")
	}
	return end, nil
}
